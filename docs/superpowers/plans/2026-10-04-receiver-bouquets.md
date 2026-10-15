# Receiver Bouquets Implementation Plan

**Goal:** Safely edit registered receiver TV bouquets alongside Dreaminator lists.
**Architecture:** Enigma file editor with opaque entry IDs and revisions; App serializes writes using applyMu; dedicated HTTP endpoints and a separate browser module render receiver drafts.
**Tech stack:** Go standard library, embedded vanilla JavaScript, existing CSS.

- [x] Add `internal/enigma/receiver_test.go`: fixture contains DVB, IPTV, marker, extra directives, a managed bouquet and a missing file. Assert listed external files only, byte-preserving reorder/removal, revision conflict, backup undo, copy index preservation, traversal/symlink rejection and managed Apply coexistence. Run `go test ./internal/enigma` before implementation to establish missing feature failure.
- [x] Add `internal/enigma/receiver.go`: Reader/Save/Undo/Copy operations constrained to registered files; atomic writes, private undo snapshots, SHA-256 revision guards. Resolve optional DVB labels through `WebIf.Services` without exposing service URLs to the UI.
- [x] Add `internal/app/receiver.go` and `internal/server/receiver.go`: endpoints GET list/detail, PUT selected draft, POST undo/copy/reload. Serialize file operations with applyMu. Return reload warnings separately from saved data so retries cannot duplicate writes.
- [x] Add endpoint integration coverage using temporary E2/data directories and fake OpenWebif; verify reload and stale-save responses. Run focused tests, then `make test`.
- [x] Add `web/static/receiver.js` using existing DOM/API helpers passed through a factory. Lists render two sections; receiver detail keeps a local draft across background state polls, exposes name input, reorder/remove, Save, Discard, Copy and Undo; warns about external provider refreshes.
- [x] Add source-card guidance linking to channel groups and explaining Send to TV; manually verify it and receiver edit/save/undo/copy in a scratch local server through the browser.
- [x] Build/install next local package revision, preserve all receiver data, verify receiver API/UI and unchanged hashes of provider files. Update README/ROADMAP and record checks. No GitHub push requested.

## Verification results

2026-10-04: `make test` (Go vet, all Go packages, Node navigation regression) passed. Receiver tests cover unchanged service blocks, invalid/stale edits, symlinks, copy coexistence, failed-save undo retention, 13 MiB snapshots and permission preservation. Browser scratch receiver verified rename, remove, reorder, save, warning on unavailable OpenWebif, undo, copy and draft retention across navigation. Independent review findings were fixed and rechecked.

Installed local `0.6.0-r3` on DM520 and opened existing Cyrius bouquet through the new UI without editing it. API lists 32 foreign index entries, of which 31 have files; LastScanned is a stale reference. All 31 original foreign bouquet checksums match the pre-import baseline. Current managed state (17 bouquets, 1303 selected channels) matches the pre-r3 backup; earlier in the session 26 groups / 1802 channels were imported and verified in OpenWebif.
