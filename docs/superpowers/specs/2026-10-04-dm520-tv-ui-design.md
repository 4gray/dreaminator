# TV catalog for DM520: native full-screen shell, TMDB, search

Approved by the user on 2026-10-04 (mockups: claude.ai artifact "Dreaminator TV — mockups"). Builds on the Stalker catalog MVP (`2026-10-04-stalker-catalog.md`).

## Constraints that shaped the design

Measured on DM520 / OpenHDF 6.4 build 137 / Python 2.7.15 / FullHDLine skin (1920×1080), two hardware threads, 312 MB RAM, live IPTV usually playing through exteplayer3 (30–90 % CPU):

| Finding | Consequence |
|---|---|
| `eListbox` has no grid or horizontal mode on 6.4 | Grid and lists are fixed native widgets driven by Python models (`ui_models`) |
| Go poster scaling (soft-float MIPS): 18–73 s per poster | Go never decodes catalog pictures; JPEG/PNG pass through, `ePicLoad` (C++, own thread) decodes and scales |
| `ePicLoad.getData()` costs 20–50 ms on the GUI thread | Decoding and conversion wait until keys are quiet for 250 ms; one picture at a time; visible cards + one row ahead |
| Alpha-blending a full-size PNG over a decoded picture stalls the GUI 1–2 s | Placeholders are flat labels; rounded corners are four 10 px masks; focus ring is four strips; no overlays on images; the card backdrop has its fade baked in by Go (integer YCbCr maths, ≈0.7 s once, service at `nice 10`) |
| 20 KB catalog JSON takes 50–540 ms to parse in Python 2 (GIL held) | List pages carry lean cards (≈2.5 KB); long texts only via `/details` |
| `Screen` is a `dict`: an attribute named `items` crashed Enigma2 on close | Never shadow dict methods; the harness fake `Screen.close` walks `self.items()` like GUISkin |

Grid size was chosen after measuring 5×2 (216×324) and 6×2 (184×276) with live TV playing: both under 140 ms per key with zero stalls; 6×2 halves per-poster conversion and shows 12 titles. Final: 6×2, 192×288.

## Layout and focus

Full-screen, borderless (`wfNoBorder`), 1920 canvas scaled for HD; content inside 80/48 px safe margins. Palette from IPTVnator's dark graphite theme, one accent (`#78adff`) for focus everywhere; Geist Regular/SemiBold bundled (OFL).

Zones: rail → categories → content (poster grid, or channel list for Live TV); in search: rail → keyboard → scope row → results. Arrows move focus only; left at the first column moves to the previous zone; right at the last column does nothing. CH±, `<`/`>`, PgUp/PgDn turn a whole screen. Back climbs: content → categories (search: keyboard) → rail → close. Positions are kept per (portal, section, category). Cards are separate screens; Back closes them and the grid keeps its focus. Colour keys: green sources (portals and playlists), yellow refresh, blue search; Menu opens the status/QR screen. OK acts on release; holding it (`okLong`, about a second) adds the focused channel, film or series to the favorites or takes it out (v0.10, chosen by the user over TEXT and “0”); the film card also has a “Add to favorites” button.

Rail (v0.10): “Favorites” and “Recents” above the source, each with the categories “Channels” (channel list) and “Movies and series” (grid). Favorite marks: a 30 px star at the end of a channel row, blended over the row only; an opaque 34 px tile in the poster's top-right corner (no blending over a decoded picture). Both are toggled through cached widget state, so scrolling changes visibility only where a row's mark changes.

Rail: collapsed icons (152 px), expands to labels when focused (440 px, panel drawn above content). Bottom items: Settings (existing status + QR screen), About (versions, TMDB notice, font licence).

## Data and API

- `GET /api/catalog/sources` → `[{id, name, type, sections}]`: Stalker portals have `itv`, `vod`, `series`; playlists (M3U link, uploaded file, Xtream) only `itv`, built from the channel list the service already keeps. Their channels play the playlist address with the bouquet service type; the rail hides sections a source lacks.
- `GET /api/catalog/{source}/items` → `{items: [{key, id, name, kind, year, duration, rating, hasPoster, number, fav}], page, hasMore, pageSize, total}`. `key` is the navigation key (opaque, HTTP 410 after a refresh); `id` is persistent: `<source>/<type>/<provider id>`, a season adds the season id, an episode also its number (v0.10).
- `GET /api/catalog/home` → `{favorites: [{id: itv|video, name, count}], recent: [...], list}`; `GET /api/catalog/home/items?list=favorites|recent&category=itv|video&page=` → cards of every source with `source`, `type` and `missing` (gone from the playlist). Their keys work with the source's own calls (card, seasons, pictures, play).
- `POST` / `DELETE /api/catalog/favorites` `{source, id, key}` → `{fav, channel, list, created, unsent}`: channels go to the channel list “Favorites” (made again if renamed or deleted), films and series to `library.json`. `POST /api/catalog/close` writes the library now; the plugin then sends changed favorite channels with `POST /api/apply`.
- `GET /api/catalog/{source}/details?key=` → provider entry (description, genres, ratings with source, director, actors, age, country, type) + TMDB facts when matched (`tmdb`, `hasBackdrop`).
- `GET /api/catalog/{source}/image?key=&kind=poster|backdrop` → pass-through poster, or the shaded TMDB backdrop.
- `GET /api/catalog/{source}/episodes?key=<season>` → TMDB names/overviews for episode numbers; the plugin applies them only to episodes the portal listed.
- `POST /api/catalog/{source}/play` `{id, key}` → fresh link; the id plays after a refresh too, the key is the fallback. A started row becomes the newest of the 30 recents.
- `GET /api/catalog/{source}/search?type=vod|series|itv&q=&page=` → cards + `scope`: `server` (portal `search` parameter), `channels` (full live list), `loaded` (portal ignores searching; only cached entries, with `searched` count). Support is detected once per portal section: real results are cross-checked with a nonsense query. Errors carry `code` (`network`, `unsupported`, `expired`, `input`, `missing`, `provider`).

## TMDB

User-supplied v3 key, language, test, on/off — stored by the Go service outside `Settings` (state responses only show a masked hint), never logged (sanitised URL errors). Matching: explicit portal TMDB id first; else normalised title (equivalent Cyrillic letters normalized, quality tags, `(YYYY)`, season suffixes) of name and original name, type movie/tv, year; scored with a minimum and a margin over the runner-up, otherwise "ambiguous" → provider data. Manual match/reset from WebUI, persisted per (portal, type, provider id). Bounded in-memory caches; no disk caches. Any TMDB failure falls back to provider data; playback never depends on TMDB.

## Verification

Go: search modes, ignored-search fallback, network vs empty, artwork pass-through and bounds, backdrop shading, TMDB matching/failures/key secrecy. Python (2.7 and 3, fake Enigma2): focus models, keyboard, paging, stale pages/searches/backdrops/seasons, double OK, player return, portals without Live TV, HD/FHD bounds. Simulator: API flows incl. search and artwork. DM520: appearance, key latency and stalls via `/tmp/dreaminator-ui-trace`, memory, playback round-trip — results in ROADMAP. The simulator never certifies decoding or performance.

Added in v0.9 (programme guide, see ROADMAP): live rows carry `now` (`{title, start, stop}`, no description); `GET /api/catalog/{source}/epg?key=&day=&tz=` answers now and next with descriptions, the days that have programmes and, with `day`, that day's programmes; `GET /api/catalog/{source}/now?keys=` refreshes rows on screen. EPG/Info on a channel opens the day screen (`ui_guide.py`), where `<`/`>` and CH± turn days.

## Out of scope

Xtream catalog, transcoding, external players, history/resume, downloads, EPG in the channel list, automatic next episode.
