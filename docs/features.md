# Features and remote control

[Back to the README](../README.md) · [Setup guide](installation.md) · [Roadmap](../ROADMAP.md)

## Sources

| Source | Live TV | Movies and series | Catch-up |
| --- | --- | --- | --- |
| M3U URL or uploaded file | Yes | Planned | Supported M3U archive attributes |
| Xtream Codes account | Yes | Planned | Planned |
| Stalker / Ministra portal | Yes | Yes | Planned |

M3U supports `url-tvg`, `tvg-id`, `tvg-name`, `tvg-shift`, `group-title` and `#EXTGRP`. Uploaded files stay on the receiver and a failed replacement retains the previous channel list. Movie/series entries are currently filtered out of M3U live TV.

Xtream supports provider categories, account/subscription checks, TS or HLS output and automatic provider EPG. A `get.php?username=…&password=…` URL is recognized as an Xtream account.

Stalker/Ministra accepts the provider's portal address and MAC, plus optional login, password, serial number and Device ID. It loads genres, including hidden adult genres, and portal EPG. Temporary stream URLs are resolved when playback starts. A portal can provide a movie catalog even without live channels. Regular series, episodes embedded in VOD and Ministra `is_series` seasons are supported.

## Lists and bouquets

- Add individual channels with a star, add a whole group, or make a group its own list.
- Reorder lists and channels by dragging on the tablet.
- **Send to TV** writes ordinary Enigma2 bouquets and reloads them without a receiver restart. Existing satellite bouquets are retained.
- **On TV** tunes a channel immediately and wakes the receiver from standby.
- **Receiver bouquets** provides a separate editor for external bouquets: rename, reorder, remove entries, create an independent copy, undo the last save, or delete the bouquet. Drafts require **Save to TV**; revision checks prevent overwriting external updates. Service references and separators are preserved.

Provider plugins may recreate or replace their bouquets during refresh. Edit a copy when you want an independent list.

## Programme guide and channel logos

XMLTV `.xml`/`.gz` guides are filtered to selected channels for EPGImport, reducing a large provider guide to the relevant programmes. Matching tries `tvg-id`, then normalized name and `tvg-name`, ignoring common quality suffixes and treating equivalent spelling consistently. Multiple guides are used in priority order; `tvg-shift` supports time-shifted channels.

The TV catalog has now/next panels, progress bars and a daily programme screen. It also covers channels outside your bouquets: a compact 36-hour XMLTV index for M3U and on-demand short guides for Xtream/Stalker. A missing guide and a failed request have different messages.

Playlist logos (`tvg-logo`, Xtream `stream_icon`, Stalker `logo`) become **220×132 Enigma2 picons** for selected channels. Dreaminator reuses the directory Enigma2 already searches, or falls back to `/usr/share/enigma2/picon`. It refreshes after **Send to TV** and overnight, skips unchanged logos and removes only files listed in its own manifest. PNG, JPEG, GIF and WebP are supported. Dark logos receive a light backing for readability.

## Native TV catalog

The Enigma2 plugin uses native widgets with a dark theme, Geist typography and a single focus accent. The left navigation rail expands on focus. Sections include source selection, Live TV, Movies, Series, Search, Favorites, Recents, Settings and About. Unsupported sections are hidden for that source. The last source, section and focus are retained during the Enigma2 session.

Movies and series use a **6×3 poster grid**. The selected title, year, rating and duration appear in the header. Missing artwork shows a title placeholder; series have a badge. **Red** changes sort order: newest, popular, rating or title. Seasons and episodes retain provider order.

Movie cards show provider and optional TMDB information, including labeled rating sources, descriptions, genres, director and cast. Series cards retain the portal's exact season/episode hierarchy; TMDB enriches only episodes actually supplied by the portal. Search uses a Russian/English/numeric on-screen keyboard and stays within the selected source and section. The status explains whether results came from portal search, the complete live channel list or only loaded pages when the portal ignores search.

Playback uses Enigma2's native player, normally service type **4097**. Playlist live channels use the same service type as their bouquets. Stalker obtains a fresh link for the exact selected episode; double OK does not request a second link. After stopping, the catalog restores its previous screen/focus and the channel that was playing before. Early failures show a channel/recording error instead of silently returning.

### Remote control

| Button | Action |
| --- | --- |
| Arrows | Move focus between navigation, categories and content |
| OK | Open or play; short OK acts on release with the Dreaminator keymap |
| Hold OK, about one second | Toggle favorite; the card also has a favorite button |
| Back / Exit | Go up one level while keeping position, then close |
| CH+ / CH−, previous / next | Change page; change day in the programme guide |
| Red | Sort movies/series; restart the current programme in the guide |
| Green | Choose source |
| Yellow | Refresh |
| Blue | Search |
| Menu | Settings, status and QR code |
| Info / EPG | Movie details or the selected channel's daily guide |
| OK on a past programme with an archive | Play that recording |

Physical remote behavior still needs the hardware checks listed in the roadmap. OpenWebif can send long presses, but that does not verify a physical remote's long-press events.

## Favorites and recents

Favorite channels use the same Favorites list as the tablet. If it is renamed or deleted, the TV catalog recreates the expected list. Closing the catalog after changing favorite channels sends the channel lists to TV. **This also sends any pending tablet list edits**, with notices in both interfaces.

Movie/series favorites and the **30 most recent items** are stored in `library.json`, separately from main settings. Favorites support up to 500 video entries. Writes are batched, normally no more than once a minute after the initial delayed save, and flushed when closing the catalog or stopping the service. Stalker saved entries retain enough provider data to reopen films/seasons after a service restart. Missing playlist channels are marked and can be removed from favorites.

Recents track what was launched; playback position, watched flags, resume prompts and automatic next episode are planned.

## M3U catch-up

The programme screen plays available past programmes with **OK** and starts the current programme from the beginning with **red**. The service builds the archive URL; the plugin sends the channel and programme start time. The player returns to the same guide row when closed.

Supported M3U modes are `shift`, `append`, `default`, `flussonic` and `xc`, with `catchup`, `catchup-type`, `catchup-days`, `catchup-source`, `tvg-rec` and `timeshift`. Archive metadata alone can imply `shift`; the maximum retained depth is 14 days. This M3U `xc` URL mode does not mean Xtream account archive integration is implemented.

Past programme titles accumulate during nightly XMLTV passes and survive restarts in `guide-archive.gob`. Provider retention, archive flags and actual recordings may differ. Native seeking depends on whether the provider returns finite VOD or a sliding HLS playlist. Real provider URLs and receiver GStreamer ingestion have been checked; the remaining picture, sound, seeking and physical-remote checks are listed in the roadmap.

## Checks and optional metadata

**Check playlist** shows Xtream/Stalker subscription status, expiry and plan information, or tests M3U URL availability. Subscription status also refreshes with playlists; a warning appears within seven days of expiry.

**Check channel** probes the stream from the receiver, including Stalker resolution and HLS segment access. It reports response time, format, video/audio codecs and download speed. MPEG-TS tables provide codec information without ffmpeg. A probe occupies one provider connection and runs only when requested. fMP4, DASH and receiver codec limits are reported as compatibility cautions, not certified playback results.

Optional TMDB integration supplies localized descriptions, ratings, cast photographs, backdrops, episode stills and missing posters. Explicit provider TMDB IDs take priority; otherwise matching uses normalized titles, year and media type, with ambiguity falling back to provider data. Manual corrections are available in the web interface. See [setup](installation.md#optional-tmdb-metadata).

*This product uses the TMDB API but is not endorsed or certified by TMDB.*

## Current limits

- Application labels currently use Russian; this documentation is in English.
- Other receivers/images and HD rendering are not yet hardware-certified.
- Xtream/M3U movies and series, Xtream/Stalker archive integration, cross-source search and search history are planned.
- Playback resume, watched tracking, automatic next episode and downloads are planned.
- Manual EPG matching and `.xz` guides are not available.
- There is no transcoding or media proxy. Containers, codecs, audio, subtitles and seeking depend on Enigma2 and the provider. DASH is unverified.
- The web interface is intended for a trusted local network and has no login.
