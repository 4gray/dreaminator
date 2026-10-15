# Historical roadmap

This snapshot records the plan at release time. See the [current roadmap](https://github.com/4gray/dreaminator/blob/main/ROADMAP.md) for later implementation and hardware results.

## v0.2 - MVP (complete)

- M3U and Xtream live TV, VOD filtering, uploaded playlists retained on the receiver.
- Custom lists, stars, drag ordering, Send to TV, immediate tuning and standby wake.
- XMLTV filtering by tvg-id, name and tvg-name; multiple sources in priority order; header/channel tvg-shift.
- Overnight refresh, installable package and boot startup.

## v0.3 - TV plugin (complete)

- Plugins/extensions entry with tablet address and QR code, playlist/EPG status, apply, refresh and service restart.
- `/api/summary`, `/api/qr.png`, `/api/refresh-all`; address in web settings.
- Python 2.7/3 harness against the virtual receiver.

## v0.4 - Stalker live TV (complete)

- Portal/API discovery, MAC and optional login/password/SN/provider-supplied Device ID; no automatic Device ID generation.
- MAG250 identity, handshake/profile, status handling, retained token/API URL and reauthentication on authorization errors.
- Full/paginated channel loading, including hidden genres.
- Local resolver with fresh create_link or static cmd, 302 player redirect and get_events watchdog.
- Portal EPG when no explicit guide URL is configured; fake portal and simulator fixtures.

## v0.5 - Checks (complete)

- Xtream/Stalker subscription status, plan/expiry and seven-day warnings.
- Stream probe without ffmpeg: response time, format, PAT/PMT codecs, download speed and compatibility cautions.

## v0.6 - Picons (complete)

- Selected channel logos converted to 220x132 service-reference picons.
- Reuse the receiver's active picon directory, otherwise /usr/share/enigma2/picon.
- Own-file manifest, unchanged-logo caching, preservation of external picons and light backing for dark artwork.
- Hardware appearance and flash usage still need verification at this release.

## Simulator verification

- [x] Same package runs on QEMU MIPS32R1 without FPU (-cpu 4Kc).
- [x] HTTPS works without system certificates through embedded Go roots.
- [x] Strict bouquet validation, satellite bouquet preservation, immediate tuning and standby wake.
- [x] 178 MB XMLTV filtering in about 17 seconds under emulation, Go memory about 20-25 MB; EPGImport simulation maps programmes.

## Real DM520 checks outstanding at this release

- [ ] Startup on Linux 3.4/BMIPS4380; bouquet reload without GUI restart.
- [ ] Real player codecs, audio and redirect support; ServiceApp alternatives if necessary; DASH is unverified.
- [ ] Installed EPGImport channel-reference/configuration compatibility and automatic source selection.
- [ ] Service memory alongside Enigma2.
- [ ] Plugin menu/extensions entry and icon after GUI restart; FullHDLine layout, button colors and QR camera scan.
- [ ] Displayed address matches eth0 system information.

## Real portal checks outstanding

- [ ] GStreamer/4097 and exteplayer3/5002 follow the resolver redirect.
- [ ] Player respects the User-Agent suffix; determine whether stream cookies/Bearer are required.
- [ ] Check contention when another device uses the same MAC.

## Follow-up plans

- Manual XMLTV channel mapping when playlist/guide identifiers and names differ.
- Explain that overnight/catch-up synchronization also sends pending tablet edits.
- Tablet movie/series catalog with posters and Watch on TV, excluding VOD from bouquets.
- .xz EPG archives and per-stream EXTVLCOPT User-Agent/Referer support.
- Existing satellite bouquet import/editing, optional Android WebView/discovery and private-VPN support.

## Original Stalker resolver plan

Temporary portal links cannot be stored as permanent bouquet URLs. A source carries portal address, MAC and optional identity fields. The client performs handshake/profile, genre/channel loading and pagination, with MAG headers/cookies and authorization renewal. Bouquets point to the local /stalker/{source}/{channel} resolver; each tune calls create_link and redirects the player, keeping media off the service. Cache sessions/links briefly, respect connection limits, and feed portal EPG into the existing guide pipeline. IPTVnator provides a protocol implementation reference.
