# Stalker catalog MVP implementation

Approved by the user on 2026-10-04. Sources configured through WebUI appear in the native Enigma2 plugin independently of bouquet selections. Stalker first; Xtream remains Live TV bouquet-only in this release.

## Components

- Go Stalker client: categories, paginated catalog, regular series, embedded VOD pseudo-season and lazy Ministra seasons/episodes; fresh create_link with exact episode selection.
- Application/API: opaque source-scoped navigation keys, separate metadata/poster caches, existing live channel cache, background network requests, safe playback headers.
- Python 2.7/3 plugin: source/section/category/item browser, artwork, native MoviePlayer 4097, return to list and previous TV; settings/QR screen retained.
- WebUI: explain source visibility vs explicitly selected Live TV bouquets.

## API

GET /api/catalog/sources returns Stalker source IDs/names only. GET /api/catalog/{source}/categories takes type and optional refresh=1. GET /api/catalog/{source}/items takes type, category, parent key, page, optional refresh=1; returns items, page, hasMore. GET /api/catalog/{source}/poster takes key and returns resized PNG. POST /api/catalog/{source}/play takes JSON key, returns URL, name, serviceType4097, scoped headers. Commands, portal identity and tokens are absent from catalog list payloads.

## Limits and lifecycle

32 cached pages, 2048 navigation records, 4MiB metadata plus 4MiB posters per source. Page/artwork TTL10min; bounded navigation records survive long playback. Source credential changes, deletion and channel refresh invalidate keys. HTTP410 asks the UI to navigate back and reopen. Poster decode is serialized; provider catalog response limit8MiB; synthetic episode pages50. Movies never become bouquet channels.

## Verification

Run make test, Go race tests on app/server/stalker, make sim and make plugin-test (Python3 and2.7 plus real simulator API). Test >500 live channels, all three series modes, selected episode, fresh links, token renewal, repeated pages, malformed payloads, network interruptions, source isolation and VOD-only portal, bounded memory, stale callbacks, double OK, native exit and no URL-history persistence. Native DM520/OpenHDF validation must be reported separately; simulator cannot certify codec/audio/seek support.

## Deferred

Xtream catalog, catch-up, search, favorites/history/resume, downloads, automatic next episode, transcoding, media proxy. No PR/push is required for this local prototype. If pushed later, run both local reviewers under local-review-before-pr on the committed branch before every push.
