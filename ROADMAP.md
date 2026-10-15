# Historical roadmap

This snapshot records the plan at release time. See the [current roadmap](https://github.com/4gray/dreaminator/blob/main/ROADMAP.md) for later implementation and hardware results.

## v0.2 - MVP (complete)

- M3U and Xtream live TV, VOD filtering, uploaded playlists retained on the receiver.
- Custom lists, stars, drag ordering, Send to TV, immediate tuning and standby wake.
- XMLTV filtering by tvg-id, name and tvg-name; multiple sources in priority order; header/channel tvg-shift.
- Overnight refresh, installable package and boot startup.

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

## Follow-up plans

- Manual XMLTV channel mapping when playlist/guide identifiers and names differ.
- Explain that overnight/catch-up synchronization also sends pending tablet edits.
- Tablet movie/series catalog with posters and Watch on TV, excluding VOD from bouquets.
- .xz EPG archives and per-stream EXTVLCOPT User-Agent/Referer support.
- Existing satellite bouquet import/editing, optional Android WebView/discovery and private-VPN support.

## Planned Stalker integration

The portal integration was planned for the next milestone; it is not implemented in this snapshot.

## Original Stalker resolver plan

Temporary portal links cannot be stored as permanent bouquet URLs. A source carries portal address, MAC and optional identity fields. The client performs handshake/profile, genre/channel loading and pagination, with MAG headers/cookies and authorization renewal. Bouquets point to the local /stalker/{source}/{channel} resolver; each tune calls create_link and redirects the player, keeping media off the service. Cache sessions/links briefly, respect connection limits, and feed portal EPG into the existing guide pipeline. IPTVnator provides a protocol implementation reference.

- Channel-name-based picons remained planned in this snapshot.
