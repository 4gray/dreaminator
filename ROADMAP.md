# Roadmap

[README](README.md) · [Features](docs/features.md) · [Development](docs/development.md)

This document separates implemented features, simulator checks and real receiver validation. The current source package is **0.11.1-r4**, combining the EPG-key integration, EPGImport IPTV references and separate Dreambox/Vu+ builds. Completed implementation does not imply that every playback or physical-remote check has passed.

## Release overview

| Version | Scope | Status |
| --- | --- | --- |
| 0.2 | M3U/Xtream live TV, lists, XMLTV, nightly refresh | Implemented |
| 0.3 | Native plugin, status and QR code | Implemented |
| 0.4 | Stalker/Ministra live TV | Implemented |
| 0.5 | Subscription and stream checks | Implemented |
| 0.6 | Channel picons | Implemented; latest historical GitHub release |
| 0.6.0-r3 | Existing receiver bouquet editor | Implemented and installed on DM520 |
| 0.7 | Stalker movie/series catalog | Implemented; hardware checks below |
| 0.8 | Full-screen TV shell, search, TMDB, playlist live TV | Implemented and measured on DM520 |
| 0.9 | Native catalog programme guide | Implemented and installed; further hardware checks pending |
| 0.10 | Favorites and recents | Implemented and installed; further hardware checks pending |
| 0.11 | M3U catch-up | Implemented and installed; picture/audio/seeking checks pending |
| 0.12 | Xtream and M3U movies/series | Planned |
| 0.13 | Playback progress, watched flags and resume | Planned |

## v0.2–v0.6: foundations

### Implemented

- M3U URLs and uploaded files, Xtream live TV, exclusion of VOD entries from live lists.
- Stars, custom lists, drag ordering, Send to TV and immediate zap/wake.
- XMLTV matching by `tvg-id`, name and `tvg-name`; multiple URLs in priority order; channel/header `tvg-shift`.
- Overnight refresh, `.ipk` packaging and boot startup.
- Native plugin in Plugins/extensions: receiver address, QR, status, apply, refresh and service restart; `/api/summary`, `/api/qr.png`, `/api/refresh-all`.
- Stalker API discovery from `/c/`, `portal.php` or `server/load.php`; MAC, optional `do_auth`, SN and provider-supplied Device ID.
- MAG250 headers/cookies/prehash, handshake/profile, status 0/1/2, retained API/token and reauthentication on authorization errors.
- `get_all_channels`, paginated `get_ordered_list` and hidden genres.
- Local Stalker resolver, fresh `create_link` or static `cmd`, 302 redirect, watchdog `get_events`, MAG User-Agent service suffix.
- Portal EPG via `get_epg_info`/`get_short_epg` when no custom guide is configured.
- Subscription checks with a seven-day expiry warning; HTTP/HLS probes and PAT/PMT codec detection without ffmpeg.
- 220×132 picons in the receiver's active directory, managed-file manifest, unchanged-logo caching and light backing for dark logos.

### Verified in the simulator

- [x] Package executable on QEMU MIPS32R1 without an FPU (`-cpu 4Kc`).
- [x] HTTPS playlist/EPG downloads without system certificates, using embedded roots.
- [x] Strict bouquet parsing, preservation of satellite bouquets, zap and standby wake.
- [x] 178 MB XMLTV filtered in approximately 17 seconds under emulation, Go memory around 20–25 MB; simulated EPGImport maps services.
- [x] Python 2.7/3 plugin harness against the virtual receiver.

### DM520 baseline, 2026-10-04

- [x] Executable starts on Linux 3.4 / BMIPS4380 when built with Go 1.26.8. Go 1.26.0 crashed in futex startup.
- [x] OpenWebif `servicelistreload?mode=2` loads lists without restarting the GUI: 26 bouquets and 1,802 channels verified.
- [x] Plugin entry/icon, FullHDLine status screen, button colors and QR drawing; displayed address matches `eth0`.
- [x] Idle service heap approximately 0.7 MB, system memory approximately 5.9 MB; loaded measurements still needed.
- [x] On Vu+ / openATV 7.5, native EPGImport accepts full IPTV references: 4,278 events imported; native EPG verified on 83 of 89 mapped channels (six had no programmes in current XMLTV).
- [ ] Verify automatic source selection on a fresh EPGImport installation; the tested Vu+ source was selected through its UI.
- [ ] Confirm the extensions/blue-menu entry and scanning the QR with a tablet camera.
- [ ] Confirm picons in the channel list/infobar and flash usage.
- [ ] Test real provider redirect/header requirements and same-MAC session contention.

Installation scripts need an explicit system `PATH`: noninteractive OpenHDF SSH otherwise omits `start-stop-daemon`, `update-rc.d` and `ip`. The plugin must not replace `Screen.show()` with an API callback. Lifecycle tests cover this on Python 2.7 and 3.

Simulator lessons: unrelated playlist/guide identifiers may need manual EPG matching, which remains planned. Overnight/catch-up synchronization also applies pending tablet edits. Since v0.10, closing the TV catalog after changing favorite channels has the same effect; both interfaces explain it.

## Receiver bouquet editor, local 0.6.0-r3

- [x] External registered TV bouquets, including DVB, IPTV, markers and missing files, shown separately.
- [x] Draft rename/reorder/removal, explicit save, independent copy, last-save undo, whole-bouquet rename/deletion.
- [x] Revision checks, atomic writes, private recovery snapshots and unchanged original service blocks.
- [x] File/API tests and browser verification on a scratch receiver, including unavailable OpenWebif and retained drafts.
- [x] Installed 2026-10-04: 31 external bouquet files and one missing LastScanned reference. All original file checksums matched the pre-import baseline.

See the [design](docs/superpowers/specs/2026-10-04-receiver-bouquets-design.md) and [implementation plan](docs/superpowers/plans/2026-10-04-receiver-bouquets.md).

## v0.7: Stalker catalog MVP

- [x] Web-configured sources appear independently of selected bouquets.
- [x] Live TV, VOD, ordinary series, embedded VOD episodes and lazy Ministra seasons.
- [x] Source-scoped opaque navigation keys, paginated categories/items, poster and playback API; HTTP 410 requests reopening expired navigation.
- [x] Native MoviePlayer/4097, fresh exact-episode links, safe portal-scoped headers, return focus and restoration of previous TV service.
- [x] Bounded caches, serialized artwork decoding and network work outside the GUI/application mutex.
- [x] Tests for all series modes, >500 live channels, repeated pages, token renewal, VOD-only portals, credential changes, stale callbacks, double OK and player return.
- [x] DM520/OpenHDF 6.4 build 137/Python 2.7.15: screens open without a green screen, real portal categories/movie/selected episode work (2026-10-04, using the v0.8 shell).
- [x] Synthetic MKV/MP4/TS over HTTP with Range, H.264 Baseline 640×360/AAC: start, picture, pause, forward seek, focus and previous-channel restoration.
- [ ] Listen to HDMI audio; validate real HD/HEVC streams and additional audio codecs.
- [ ] Measure memory with a large catalog.

The MVP did not include search, catch-up or history. Later milestones add some of these; resume, downloads, automatic next episode, transcoding and media proxy remain outside the current implementation.

## v0.8: full-screen shell, search and TMDB

The [design specification](docs/superpowers/specs/2026-10-04-dm520-tv-ui-design.md) records the hardware constraints and APIs.

- [x] Native navigation rail, categories, final 6×3 poster grid, live list, detail cards, on-screen keyboard, source picker and About.
- [x] Geist fonts (OFL), one focus accent, Full HD canvas and scaled HD layout.
- [x] Arrow focus, page keys, hierarchical Back, color actions and overlay menus.
- [x] JPEG/PNG pass-through, native threaded `ePicLoad`, one decode at a time after 250 ms of key silence; visible posters plus one row, ≤10 MiB decoded artwork in RAM, temporary files in `/tmp`.
- [x] Lean catalog pages and details fetched separately; pre-shaded TMDB backdrop; service priority `nice 10`.
- [x] Portal search with an ignored-parameter probe; full live-channel search; explicit loaded-page fallback; network/unsupported/empty states distinguished.
- [x] User TMDB v3 key, metadata language, test/disable/delete, manual match/reset, provider fallback and key secrecy.
- [x] Localized title, genres, ratings, cast photos, director/creators, episode stills/descriptions; portal-provided episode hierarchy retained.
- [x] Sort overlay (newest default, popular, rating, title); source/section choice retained during the Enigma2 session.
- [x] All playlist types in Live TV catalog; only supported sections shown; early stream failure explained.

### Measurements on DM520

OpenHDF 6.4 build 137, Python 2.7.15, FullHDLine 1920×1080. Live IPTV through exteplayer3 used 30–90% CPU. Key latency was measured to the next Enigma2 main-loop iteration, 2026-10-04/05.

| Measurement | Result / change |
| --- | --- |
| Original Go poster pipeline | 18–73 seconds/poster at 100% CPU; replaced with pass-through |
| Native `ePicLoad` | 85–300 ms in its thread; `getData()` 20–50 ms in GUI |
| Full-size alpha poster overlay | 1–2 second stalls; replaced with tiny corner masks and strip rings |
| Python JSON parse | 20 KB: 50–540 ms; lean ≈2.5 KB cards: 5–28 ms |
| 5×2 grid | Median 43 ms, p90 125 ms, max 136 ms |
| 6×2 grid | Median 51 ms, p90 98 ms, max 135 ms |
| Final 6×3 grid | Median 47 ms, p90 156 ms, max 167 ms; no stalls >300 ms |
| Full 18-poster redraw | Approximately 100 ms; cached widget state avoids repeated ≈2.8 ms show/hide calls |
| Live list with logos | Median 41 ms, p90 137 ms, max 250 ms on a 1,802-channel playlist; RSS ≈89 MB |
| Catalog first-open RSS | 86→93 MB; after fixing retained `ePicLoad` signal, four open/browse/close cycles: 96.7→96.9 MB |
| Go backdrop preparation | Approximately 0.7 seconds once (decode 210, shade 210, encode 235 ms) |

- [x] Real portal with 16,644 movies and 395 channels: categories, posters, detail ratings, embedded series, server search and playback.
- [x] Real TMDB key: backdrop, ratings, localized genres, cast, director, series creators and descriptions.
- [x] Playlist playback restores the previous TV channel; source type labels and portal sections correct; HTTP 503 produces an early-failure message.
- [x] Fixed source-switch position stored under the wrong source and status text overlapping filled lists.
- [ ] HD 1280×720 on real hardware; physical remote; HDMI sound.

A separate power-loss incident exposed an incomplete `state.json` on UBIFS. Atomic writes now synchronize file and directory, retain `.bak` and preserve corrupt files as `.broken-…` without preventing startup. Source/list data were recovered from the readable state and stable bouquet identifiers. This was independent of catalog playback.

## Shared decisions for v0.8–v0.13

The Go service owns parsing, search, EPG, favorites and future progress. The Python plugin remains a thin native client because parsing large JSON responses blocks Enigma2's GUI. All source types share `/api/catalog/{source}/…` and declare their supported sections.

Persistent IDs are separate from cache navigation keys: source/type/provider ID, with season/episode components as needed. Future progress is keyed by that ID, never a temporary or credential-bearing playback URL. `CatalogPlayer` does not save Enigma2 resume bookmarks or change `last_movie_played`. Library data uses a separate atomic `library.json`, with bounded write frequency.

The target remains p90 scrolling around 150 ms and no stalls over 300 ms. Keep bounded metadata caches, lean pages and ≤10 MiB decoded artwork; large future VOD catalogs must be paged from disk rather than loaded completely into RAM.

### Remaining playlist/search work

- [ ] Search all sources, grouped by Channels/Movies/Series, with exact/prefix/word/substring ranking and all query words matched.
- [ ] Retain the ten most recent searches.
- [ ] Add stream checking to the TV UI without taking an already assigned remote key.
- [ ] Test result ordering and a 5,000-channel group on DM520; target search under 0.5 seconds and stable memory after close.

The current search is source/section-specific. M3U movies moved to v0.12. Remote bindings are documented in [Features](docs/features.md#remote-control); new milestones must preserve existing actions.

## v0.9: programme guide

Implemented 2026-10-05. The catalog guide comes from the service, rather than `eEPGCache`, so channels outside bouquets can have programme information.

- [x] Selected channels indexed from filtered `epg.xml`, with names/times in RAM and descriptions read by file offset on demand; compatibility mapping from `epg-map.json` or older EPGImport files.
- [x] Xtream `get_short_epg` with base64 fields and timestamp fallback; Stalker `ShortGuide`; on-demand 15-minute cache, ≤1 MiB total.
- [x] Unselected M3U channels indexed for 36 hours in the same XMLTV pass, stored once per pass in `guide-playlists.gob`, including playlists with no selected channels.
- [x] Catalog/search `now` fields; `/epg` now/next/day with TV timezone offset; `/now` batches up to 60 visible rows. Memory is reported in `/api/debug`.
- [x] Live-row title/progress, right-side now/next details after 400 ms of idle focus, stale-response rejection and minute updates outside playback.
- [x] Daily guide with EPG/Info, day paging, current-programme focus, OK playback, Back restoration and retry.
- [x] Missing guide distinguished from failed loading; Go/Python 2.7/3 and simulator integration checks.
- [ ] Manual EPG matching.

### Index benchmarks, 2026-10-05

Synthetic 184 MB XMLTV: 3,000 channels, seven days, 527,000 programmes. The extra index covers 2,700 unselected channels.

| Work | M1 Pro | QEMU MIPS32 | DM520 estimate |
| --- | --- | --- | --- |
| Existing nightly pass | 2.7 s; 819 MB cumulative allocation | 23 s | ≈110 s |
| Pass plus 36-hour index | 2.9 s (+7%); 882 MB cumulative allocation | 24 s (+8%) | ≈120 s |
| Extra index retained in memory | 2.2 MB; 104,622 programmes | 2.1 MB | ≈2 MB |
| Read 103 MiB filtered EPG, 2,000 channels | 0.10 s; 1.7 MB | 1.4 s | ≈4 s |

DM520 timing estimates came from other measured Go operations, not a receiver benchmark. Skipping out-of-window programme bodies reduced indexing overhead from the first implementation's +28%/+41% to +7%/+8%. A failed nightly download retains the previous guide.

### Hardware checks

- [x] Installed 2026-10-05: existing 62 MB filtered EPG indexed into 552 channels/29,478 programmes/1.3 MB in under 25 seconds; service heap 4.9 MB.
- [x] First live page: 45/50 rows with now, 10.8 KB in 0.14 s; guide request 0.11 s, descriptions on 10/15 programmes.
- [ ] Measure Python parse time for larger guide-bearing pages and scrolling/progress under live playback.
- [ ] Investigate empty short-guide responses from the tested real Stalker portal; test a real Xtream panel.
- [ ] Check additional playlists after overnight indexing, guide timezone/midnight, physical EPG/Info, `eSlider` colors and `setVAlign` fallback.
- [ ] Verify guide-screen memory after four open/close cycles and only expected nightly flash writes.
- [ ] Run real XMLTV/nightly benchmarks on the receiver.

Cross-compile the benchmark with `GOOS=linux GOARCH=mipsle GOMIPS=softfloat go test -c -o epg.test ./internal/epg`. Run `nice -n 10 ./epg.test -test.run '^$' -test.bench Filter -test.benchtime 1x`; for the existing guide use `EPG_READ_FILE=/etc/dreaminator/epg.xml` and `-test.bench ReadGuide`. Do not place large temporary fixtures in RAM-backed `/tmp`.

## v0.10: favorites and recents

Implemented and installed 2026-10-05 as 0.10.0-r1.

- [x] Hold OK toggles favorites; short OK on release, press resets hold state, repeated hold events ignored. Without the custom keymap the detail-card favorite button remains available.
- [x] Stable catalog IDs and playback by `{id, key}` after refresh; source-scoped library records retain enough Stalker data to reopen saved films/seasons.
- [x] Favorite channels share the tablet Favorites list. Closing after a change applies lists, including pending tablet edits, with notices.
- [x] Up to 500 video favorites, 30 recents, atomic batched library saves, `.bak` recovery and startup/source-deletion cleanup.
- [x] Favorites/Recents above source navigation; channel list or video grid, source-aware actions and missing-channel states.
- [x] Cached star/badge visibility avoids repeated costly widget calls; no alpha blending over decoded posters.
- [x] Go, API, Python 2.7/3 and simulator tests for hold behavior, persistence, refresh, recents trimming, missing channels and apply-on-close.
- [x] DM520 service starts with existing sources; home/day-guide APIs and stable IDs work; 64 shell tests pass on installed Python 2.7.15 files, no new Enigma2 crash after GUI restart.

Stable IDs added approximately 34 bytes/row: a 50-channel page grew from 5.9 to 7.6 KB before EPG fields; an installed guide-bearing page was approximately 12.7 KB. Favorite marks add 49 widgets across shell rows/posters.

- [ ] Physical hold/release events, no extra OK after holding. OpenWebif's `command=352&type=long` checks the plugin path but not the remote itself.
- [ ] Scroll latency with guide and favorite marks, HD/FHD badge appearance, immediate bouquet reload without playback interruption.
- [ ] Stable RSS across four Favorites/detail/close cycles, library write frequency and reboot persistence.
- [ ] Saved real-portal films/series after a receiver reboot.
- [ ] Decide whether Favorites list matching should ignore emoji/punctuation rather than require the exact expected name.

For latency tracing, open the catalog deliberately with the remote, then send OpenWebif key events and inspect `/tmp/dreaminator-ui-trace.log`. Avoid navigating the plugin menu blindly: unrelated plugins may have destructive actions. DM520 `grab` does not capture the OSD in the tested setup. GUI restart is needed to load a new `keymap.xml`; development module reload does not reload it.

## v0.11: M3U catch-up

Implemented 2026-10-07; package 0.11.0-r3 installed. Past programmes play from the daily guide; red restarts the current programme. The service builds credential-bearing URLs, while the plugin submits channel/start only. Xtream account and Stalker archive support remain a second phase.

### Provider findings

| Provider fixture | Archive metadata | Observed behavior |
| --- | --- | --- |
| Edem | `tvg-rec`, no explicit mode; 1,119/3,343 channels advertise 3 or 7 days | `utc`/`lutc`; sliding HLS, 6–7 ten-second segments, no `ENDLIST`; guide supplies yesterday/today |
| VipLime | `catchup="shift" catchup-days="3" timeshift="3"`; 453/1,448 channels | Finite HLS VOD with `ENDLIST`; archive older than retention is empty; guide covers more past days |
| Cyrius | No archive metadata | No advertised archive |

Both observed archive schemes use Unix-second shift parameters. Metadata without a mode implies `shift`. `lutc` must be the **request time**, not programme end: Edem rejected the earlier implementation, while VipLime caps its own response at six hours (4,320 segments, approximately 380 KB). Past programme titles must accumulate locally because a provider's live guide may omit older archive days.

### Implemented service and plugin

- [x] M3U channel/header defaults: `catchup`/`catchup-type`, `catchup-days`, `catchup-source`, `tvg-rec`, `timeshift`; maximum 14 days, default five days for a mode without depth, explicit zero/unknown mode disables archive.
- [x] URL modes `shift`, `append`, `default`, `flussonic`, `xc`; start/end/now/duration/offset/calendar substitutions; UTC calendar, source query preservation and player header suffix handling.
- [x] Channel IDs and playlist equality unaffected by archive metadata, so refresh does not create unnecessary pending bouquet edits.
- [x] One nightly `guide-archive.gob`, titles/times only, source/channel keys with `tvg-shift`, new guide precedence and older-layer gap filling before and after truncated fresh data.
- [x] Retain eligible old indexes across startup and delayed refresh, including time-shifted archive windows; report memory through `/api/debug`.
- [x] Guide days back to −14, per-programme recording availability, `{id, key, start}` playback; unknown/future/out-of-retention requests return 404 `archive`.
- [x] Past programme emphasis, unavailable-recording explanations, OK archive/red restart, early-recording failure, duplicate-OK guard and same-row return.
- [x] Go tests for attributes/modes/substitutions, shifts, truncation, multiple nights, failed downloads, restart and playback errors.
- [x] Python 2.7/3 guide/player tests and end-to-end QEMU archive fixtures for sliding and finite HLS.

### DM520 results, 2026-10-07

- [x] 0.11.0-r1, then r2 request-time fix, then r3 review fixes installed; GUI restarted, receiver returned to standby; 68 shell tests passed with installed Python 2.7.15 files, no new Enigma2 crash.
- [x] Full refresh including downloads: 5 min 20 s, similar to earlier overnight refreshes. Archive: 1,315 channels, 54,041 programmes, 2.2 MB RAM, 1.8 MB file; reload survives service restart. Free flash was 202 MB.
- [x] Live-guide index with retained past hours: 31–40 thousand programmes, 1.5–1.9 MB.
- [x] Archive flags and past guide rows returned; future playback rejected correctly.
- [x] VipLime recordings three days old and restart links; Edem 10/14 sampled programmes on eight channels began within nine seconds of the expected time.
- [x] Receiver GStreamer accepted both providers' archive streams for 30 seconds using a fakesink.

Some channels advertise an archive that the provider does not actually supply. Edem's latest approximately five minutes may still be unavailable. GStreamer fakesink testing confirms ingestion, not picture/audio output.

### Remaining checks and second phase

- [ ] Picture and HDMI sound in CatalogPlayer/4097, alternative 5002 if necessary.
- [ ] Guide appearance, physical remote, red restart and same-row return.
- [ ] Native `getLength()`/seek with finite VOD and sliding HLS. Offset-based restart seeking is not implemented.
- [ ] Accumulation after three nights when the provider supplies only yesterday; no extra daytime flash writes.
- [ ] Xtream `tv_archive`/`tv_archive_duration`, panel timezone, timeshift URL and past `get_simple_data_table`.
- [ ] Stalker archive flags, programme IDs and `create_link` type `tv_archive`; verify against a real portal.
- [ ] Add fake Xtream/Stalker archive fixtures for those integrations.

## EPG key from bouquet channels (0.11.1)

Implemented 2026-10-11. A bouquet line remains live TV; the EPG hotkey opens Dreaminator's programme/archive screen rather than modifying native Enigma2 EPG or proxying media.

- [x] `GET /api/catalog/service?ref=` resolves a playing playlist channel. Managed bouquets match stable SID/TSID (FNV32a source/channel, ONID `1DE`) even when a URL token changes. External bouquets match decoded stream addresses; unmatched services return 404.
- [x] `/epg` accepts persistent `id`, recovering an expired navigation key.
- [x] `WHERE_EVENTINFO` entry in its own `DreaminatorEPG` folder preserves existing catalog Hotkey numbering. Its signature remains compatible with OpenHDF event menus and Hotkey discovery.
- [x] Unknown guide days step toward the nearest available day. Satellite services use native EPG immediately; IPTV lookup has a five-second timeout. Duplicate requests and late callbacks after leaving/zapping are ignored.
- [x] A live programme closes back to the current broadcast; an archive opens CatalogPlayer and returns to the guide. Info/Menu uses the selected service/event, including channels without archive.
- [x] Go service-reference parsing/matching tests, six EPG-key Python scenarios and end-to-end simulator coverage.

### DM520 remote verification, 2026-10-11

The original DM520 test build combined the EPG-key branch with the 0.11.0-r4 EPGImport channel-file fix. The current source includes both integrations and the openATV full-reference fix.

- [x] OpenHDF build 137 Hotkey bytecode/keymap matched the expected `WHERE_EVENTINFO`, `selectedevent`, path length and EPG mapping. Original native action was recorded; settings were backed up before editing with the GUI stopped.
- [x] All 42 IPTV entries in the tested bouquet resolved in 12–70 ms; both archive-enabled and ordinary channels were covered. The test receiver had no satellite services, so satellite fallback remains harness-only.
- [x] OpenWebif EPG → guide → previous day → recording (4097, 720×576) → guide → live bouquet channel. A non-archive channel used native EPG. Reopening after closing worked.
- [x] Finite VipLime HLS: seekability 3, six-hour length, number keys moved +300/+60/−60/−15 seconds and video continued at 854×480. Sliding Edem HLS reported unknown length and did not support those seeks; offset restart seeking remains planned.
- [x] Edem accumulated five past days and VipLime three; 1,339 archived channels and 149,800 programmes. No new Enigma2 crash from the installation.
- [ ] At-TV confirmation of picture, HDMI audio, guide appearance and return to live TV.

## Bouquet archive menu and Vu+ verification, 2026-10-24

Current source build **0.11.1-r4** combines the existing DM520 EPG-key work, the EPGImport IPTV-reference fix and separate MIPS/ARM packages.

- [x] **Channel list → Menu → Архив Dreaminator** opens the selected IPTV channel. openATV uses `WHERE_CHANNEL_CONTEXT_MENU`; OpenHDF uses an idempotent session-start menu adapter. The existing EPG-button action and its plugin-folder numbering are preserved.
- [x] Channel parental protection is checked before opening. Unknown/non-archive channels show an explanation; late results after closing are ignored. The archive menu's OK restarts the current programme, and future programmes stay in the guide.
- [x] All 103 Python harness checks pass under Python 2.7 and Python 3; Go vet/full tests and Node checks pass. Both packages contain identical Python plugin files, with separately validated MIPS/ARM executables.
- [x] Installed the ARM package only on Vu+ Uno 4K SE / openATV 7.5. Opened Russian Extreme HD from the actual bouquet menu, played a past programme through native 4097, observed changing video frames at 1920×1080 and advancing playback position. HLS was finite VOD; its programme-date-time matched the requested start within two seconds.
- [x] Player exit restored the same guide row. Guide exit restored the selected bouquet channel after the receiver reset its selection to the previously playing service.
- [x] EPGImport remained enabled at 05:00 after the GUI restart. Dreambox's installed package and EPG-button assignment were not changed.
- [ ] Confirm HDMI sound and physical remote behavior at the TV. The new legacy menu adapter is harness-tested on Python 2.7; its actual DM520 GUI deployment remains separate.

## v0.12: Xtream and M3U movies/series

### M3U and uploaded files

- [ ] Retain VOD separately instead of dropping it; expose `vod`/`series` catalog sections.
- [ ] Detect episodes from `/series/`, `S01E02`, `1x02` and localized season/episode names; group by normalized series title with seasons in `parent`.
- [ ] Separate `channels/<id>.vod.json` cache; never include VOD in bouquets, EPG or picons.
- [ ] Accept movie-only playlists; add parser fixtures and demo sources.
- [ ] Measure a 50,000-movie playlist using `/api/debug`.

### Xtream

- [ ] Categories/items from `get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series`; per-category loading, streamed JSON, only required fields retained, provider-response limit 8 MiB.
- [ ] Details from `get_vod_info` and `get_series_info`, lazy seasons using `parent`.
- [ ] Resolve `/movie/<user>/<password>/<id>.<ext>` and `/series/<user>/<password>/<episode>.<ext>` only for `/play`; keep credentials out of catalog lists.
- [ ] Decide between whole-catalog disk-index search and selected-category search after measuring large accounts.
- [ ] Add `internal/playlist/xtreamtest` fixtures for live, VOD, series and short EPG.
- [ ] Reuse the existing poster/detail UI and optional TMDB enrichment.
- [ ] Hardware: category latency/memory and HTTP MKV/MP4 playback.

## v0.13: progress, watched episodes and resume

Applies to Stalker, M3U and Xtream once their video catalogs exist.

- [ ] `PUT /api/catalog/progress` with persistent ID, position and duration; update RAM immediately, persist at most once a minute and on shutdown.
- [ ] `POST /api/catalog/watched` for individual items and whole seasons; manual choice overrides automatic state.
- [ ] Watched threshold 90%; started means >10 seconds and <90%.
- [ ] Continue a started episode, advance from a completed episode by season/number, or start the first episode.
- [ ] Continue-watching home row, newest first, excluding finished items; row progress/checkmarks and series completion counts.
- [ ] Resume/start-over prompt; seek only after seekability or known duration.
- [ ] Read position/length every 30 seconds, on pause, exit and EOF; reporting failures must not interrupt viewing.
- [ ] Next-episode countdown with immediate OK and Exit back to the list.
- [ ] Choose watched/season actions without conflicting with existing remote bindings.
- [ ] No progress for live streams or unknown-duration streams.
- [ ] Tests for threshold, cross-season advancement and manual precedence; hardware `seekTo`/`getLength` on MKV/MP4/HLS and flash write frequency.

## Later

- Tablet archive badges and Watch on TV for past programmes using the same `/play` start parameter.
- Tablet video favorites, resume and continue-watching; possibly a local VOD resolver and session-start observer for playback launched outside the catalog.
- Search entry from the tablet/phone keyboard.
- Manual EPG channel matching and `.xz` guides.
- Per-stream `#EXTVLCOPT` User-Agent/Referer handling.
- Add new entries to existing receiver bouquets (view/edit/remove/copy/undo already exist).
- Optional Android WebView wrapper with receiver discovery.
- Optional remote support over a private VPN.
