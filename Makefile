VERSION ?= 0.11.1-r1
HOST    ?= dm520.fritz.box
# Old Dreambox images ship an old Dropbear that only speaks ssh-rsa.
SSH_OPTS ?= -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o StrictHostKeyChecking=accept-new

LDFLAGS := -s -w -X main.version=$(VERSION)
BOX_ENV := GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0
IPK     := dist/dreaminator_$(VERSION)_all.ipk
VU_BOX_ENV := GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0
VU_BINARY  := dist/vuuno4kse/dreaminator
VU_IPK     := dist/dreaminator_$(VERSION)_vuuno4kse.ipk

.PHONY: dev test box ipk box-vuuno4kse ipk-vuuno4kse deploy logs icons ui-assets clean sim sim-logs sim-stop sim-reset docs plugin-test

## dev: run locally against a fake receiver in ./dev (no OpenWebif)
dev:
	mkdir -p dev/enigma2 dev/epgimport
	go run ./cmd/dreaminator -listen :8088 -data dev/data -e2dir dev/enigma2 -epgimport-dir dev/epgimport -plugin-dir "" -picon-dir dev/picon -webif ""

test:
	go vet ./...
	go test ./...
	node --test web/receiver.test.mjs

## box: cross-compile for the DM520 (Broadcom MIPS, little endian)
box:
	mkdir -p dist
	$(BOX_ENV) go build -trimpath -ldflags "$(LDFLAGS)" -o dist/dreaminator ./cmd/dreaminator

## ipk: installable package for opkg / the image's "install local extension"
ipk: box
	go run ./tools/mkipk -version $(VERSION) -binary dist/dreaminator -out $(IPK)

## box-vuuno4kse: cross-compile for Vu+ Uno 4K SE (32-bit ARMv7, openATV 7.5)
box-vuuno4kse:
	mkdir -p $(dir $(VU_BINARY))
	$(VU_BOX_ENV) go build -trimpath -ldflags "$(LDFLAGS)" -o $(VU_BINARY) ./cmd/dreaminator

## ipk-vuuno4kse: separate experimental package; never overwrites the DM520 build
ipk-vuuno4kse: box-vuuno4kse
	go run ./tools/mkipk -version $(VERSION) -arch vuuno4kse -binary $(VU_BINARY) -out $(VU_IPK)

## deploy: copy the package to the receiver over SSH and install it
deploy: ipk
	scp -O $(SSH_OPTS) $(IPK) root@$(HOST):/tmp/dreaminator.ipk
	ssh $(SSH_OPTS) root@$(HOST) 'opkg install --force-reinstall /tmp/dreaminator.ipk && rm /tmp/dreaminator.ipk'
	@echo "Ready: http://$(HOST):8088"

logs:
	ssh $(SSH_OPTS) root@$(HOST) 'tail -n 100 /tmp/dreaminator.log'

icons:
	go run ./tools/genicons

## ui-assets: redraw the TV catalog artwork (rings, plates, icons) from plugin/Dreaminator/ui_theme.py
ui-assets:
	go run ./tools/genui

clean:
	rm -rf dist

## sim: virtual Dreambox in Docker — real mipsel binary under QEMU + fake Enigma2
SIM_ARCH := $(shell docker version -f '{{.Server.Arch}}' 2>/dev/null || echo arm64)

sim: ipk
	mkdir -p dev
	GOOS=linux GOARCH=$(SIM_ARCH) CGO_ENABLED=0 go build -o dist/fakebox ./tools/fakebox
	cp $(IPK) dist/dreaminator.ipk
	docker build -f sim/Dockerfile -t dreaminator-sim .
	docker rm -f dreaminator-sim >/dev/null 2>&1 || true
	docker run -d --name dreaminator-sim -p 8088:8088 -p 8090:8090 \
		-v dreaminator-sim-data:/etc/dreaminator -v dreaminator-sim-e2:/etc/enigma2 -v dreaminator-sim-epg:/etc/epgimport \
		-v dreaminator-sim-picon:/usr/share/enigma2/picon \
		-v $(CURDIR)/dev:/media/dev:ro dreaminator-sim
	@echo "Dreaminator: http://localhost:8088   TV: http://localhost:8090"

sim-logs:
	docker logs -f dreaminator-sim

sim-stop:
	docker rm -f dreaminator-sim

## sim-reset: forget the virtual receiver's state (lists, playlists, settings)
sim-reset: sim-stop
	docker volume rm dreaminator-sim-data dreaminator-sim-e2 dreaminator-sim-epg dreaminator-sim-picon

## plugin-test: run the Enigma2 plugin (Python 3 and 2.7) against the running `make sim`
plugin-test:
	mkdir -p dist
	python3 tools/plugin-harness/test_lifecycle.py
	python3 tools/plugin-harness/test_catalog.py
	python3 tools/plugin-harness/test_ui_models.py
	python3 tools/plugin-harness/test_shell.py
	python3 tools/plugin-harness/catalog_integration.py
	python3 tools/plugin-harness/run.py dist/plugin-preview.html
	docker run --rm --network container:dreaminator-sim -v "$(CURDIR)":/src:ro -w /src/tools/plugin-harness \
		-e PYTHONIOENCODING=utf-8 python:2.7-slim sh -c 'python test_lifecycle.py && python test_catalog.py && python test_ui_models.py && python test_shell.py && python catalog_integration.py && python run.py'

## docs: printable PDF of the installation guide (needs Google Chrome)
CHROME ?= /Applications/Google Chrome.app/Contents/MacOS/Google Chrome

docs:
	"$(CHROME)" --headless=new --disable-gpu --no-pdf-header-footer \
		--print-to-pdf="$(CURDIR)/docs/Dreaminator-install-Windows.pdf" "file://$(CURDIR)/docs/install-windows.html" 2>/dev/null
	@echo "docs/Dreaminator-install-Windows.pdf"
