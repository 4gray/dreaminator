# Bouquet archive implementation plan

**Goal:** Open Dreaminator catch-up from native bouquets on Dreambox/OpenHDF and Vu+/openATV, then test playback on Vu+.

**Architecture:** Resolve a credential-free Enigma2 identity in Go; reuse GuideScreen and CatalogPlayer through a small asynchronous Python entry screen. Prefer the native menu hook and use an idempotent legacy menu adapter when absent.

**Tech stack:** Go 1.26.8, Enigma2 Python 2.7/3, existing plugin harness, Docker cross-builds, OpenWebif and SSH.

1. Add failing app/HTTP tests for `POST /api/catalog/bouquet` with `{"ref":"4097:0:1:SID:TSID:1DE:0:0:0:0:"}`. Cover a cold catalog cache, duplicate list membership, case normalization, unsupported references, channels without catch-up and source removal. Resolve only currently managed bouquet entries; reject collisions.
2. Implement `internal/app/bouquet_archive.go` returning `{source,item}`. Under the app mutex match `enigma.EPGRef(service.Key)`, find the source/channel, require catch-up, and register `channelItem` through the catalog cache. Add the route in `internal/server/catalog.go`; never return stream URLs.
3. Add failing Python tests for native plugin registration, the legacy menu adapter, async close/race handling and archive-only guide actions. Implement `plugin/Dreaminator/bouquet_archive.py`, register the appropriate descriptor in `plugin.py`, and add an optional archive-only argument to `GuideScreen`.
4. Run Go vet/tests, existing Node checks and Python harness tests under Python 3 and Docker Python 2.7. Build both packages with explicit version 0.11.0-r5. Request independent code review, fix findings, then commit locally. Do not push.
5. On Vu+ verify its model and idle recording status, back up its data/settings/plugin and install the ARM package only. Restart the GUI, inspect the menu and guide, play an actual past programme, verify decoder output and return to the bouquet. Recheck persisted 05:00 import and leave the receiver in a usable state. Record exact evidence and limits in ignored dist deployment notes.
