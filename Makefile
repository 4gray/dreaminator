VERSION ?= 0.4.0
HOST    ?= 192.168.178.35
# Old Dreambox images ship an old Dropbear that only speaks ssh-rsa.
SSH_OPTS ?= -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o StrictHostKeyChecking=accept-new

LDFLAGS := -s -w -X main.version=$(VERSION)
BOX_ENV := GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0
IPK     := dist/dreaminator_$(VERSION)_all.ipk

.PHONY: dev test box ipk deploy logs icons clean sim sim-logs sim-stop sim-reset docs plugin-test

## dev: run locally against a fake receiver in ./dev (no OpenWebif)
dev:
	mkdir -p dev/enigma2 dev/epgimport
	go run ./cmd/dreaminator -listen :8088 -data dev/data -e2dir dev/enigma2 -epgimport-dir dev/epgimport -plugin-dir "" -webif ""

test:
	go vet ./...
	go test ./...

## box: cross-compile for the DM520 (Broadcom MIPS, little endian)
box:
	mkdir -p dist
	$(BOX_ENV) go build -trimpath -ldflags "$(LDFLAGS)" -o dist/dreaminator ./cmd/dreaminator

## ipk: installable package for opkg / the image's "install local extension"
ipk: box
	go run ./tools/mkipk -version $(VERSION) -binary dist/dreaminator -out $(IPK)

## deploy: copy the package to the receiver over SSH and install it
deploy: ipk
	scp -O $(SSH_OPTS) $(IPK) root@$(HOST):/tmp/dreaminator.ipk
	ssh $(SSH_OPTS) root@$(HOST) 'opkg install --force-reinstall /tmp/dreaminator.ipk && rm /tmp/dreaminator.ipk'
	@echo "Ready: http://$(HOST):8088"

logs:
	ssh $(SSH_OPTS) root@$(HOST) 'tail -n 100 /tmp/dreaminator.log'

icons:
	go run ./tools/genicons

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
		-v $(CURDIR)/dev:/media/dev:ro dreaminator-sim
	@echo "Dreaminator: http://localhost:8088   TV: http://localhost:8090"

sim-logs:
	docker logs -f dreaminator-sim

sim-stop:
	docker rm -f dreaminator-sim

## sim-reset: forget the virtual receiver's state (lists, playlists, settings)
sim-reset: sim-stop
	docker volume rm dreaminator-sim-data dreaminator-sim-e2 dreaminator-sim-epg

## plugin-test: run the Enigma2 plugin (Python 3 and 2.7) against the running `make sim`
plugin-test:
	mkdir -p dist
	python3 tools/plugin-harness/run.py dist/plugin-preview.html
	docker run --rm --network container:dreaminator-sim -v "$(CURDIR)":/src:ro -w /src \
		-e PYTHONIOENCODING=utf-8 python:2.7-slim python tools/plugin-harness/run.py

## docs: printable PDF of the installation guide (needs Google Chrome)
CHROME ?= /Applications/Google Chrome.app/Contents/MacOS/Google Chrome

docs:
	"$(CHROME)" --headless=new --disable-gpu --no-pdf-header-footer \
		--print-to-pdf="$(CURDIR)/docs/Dreaminator-install-Windows.pdf" "file://$(CURDIR)/docs/install-windows.html" 2>/dev/null
	@echo "docs/Dreaminator-install-Windows.pdf"
