# Bouquet archive implementation plan

**Goal:** Open Dreaminator catch-up from native bouquets on Dreambox/OpenHDF and Vu+/openATV, then test playback on Vu+.

**Architecture:** Resolve a credential-free Enigma2 identity in Go; reuse GuideScreen and CatalogPlayer through a small asynchronous Python entry screen. Prefer the native menu hook and use an idempotent legacy menu adapter when absent.

**Tech stack:** Go 1.26.8, Enigma2 Python 2.7/3, existing plugin harness, Docker cross-builds, OpenWebif and SSH.

1. Restore the existing EPG-key/service resolver integration from preserved commit 11f59b1 and its escaped-address fix 4fa2df7. Resolve packaging conflicts by preserving the ARM/MIPS validation and packing both Dreaminator and DreaminatorEPG folders. Run the restored Go/Python tests.
2. Reuse `ChannelForService` and its existing `/api/catalog/service` route for the selected bouquet reference. Retain the EPG button and event-info behavior already installed on DM520; only add the new menu entry.
3. Add failing Python tests for native plugin registration, the legacy menu adapter, async close/race handling and archive-only guide actions. Implement `plugin/Dreaminator/ui_bouquet.py`, register the appropriate descriptor in `plugin.py`, and add an optional archive-only argument to `GuideScreen`.
4. Run Go vet/tests, existing Node checks and Python harness tests under Python 3 and Docker Python 2.7. Build both packages with explicit version 0.11.1-r4. Run local Codex review on the committed branch, fix findings and re-review. The user authorized pushing the finished changes to main and explicitly waived Greptile on 24 October 2026.
5. On Vu+ verify its model and idle recording status, back up its data/settings/plugin and install the ARM package only. Restart the GUI, inspect the menu and guide, play an actual past programme, verify decoder output and return to the bouquet. Recheck persisted 05:00 import and leave the receiver in a usable state. Record exact evidence and limits in ignored dist deployment notes.
