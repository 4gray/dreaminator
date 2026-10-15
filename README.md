# Dreaminator 0.5.0

Subscription status and stream checks from your tablet.

> Historical release. This package predates the DM520 startup fixes and current TV catalog. Follow the [current installation guide](https://github.com/4gray/dreaminator/blob/main/docs/installation.md) when choosing a package. The attached English Windows guide describes current setup guidance, not a rebuilt historical binary.

## New

- **Subscription status** on playlist cards: Xtream active/expiry/connection limit/trial details; Stalker status, expiry and plan; M3U URL availability through **Check**.
- Status refreshes with each playlist update. Within seven days of expiry, warnings appear on the tablet and in the TV plugin, which also shows the subscription end date.
- **Channel check:** the receiver opens the stream and reports response time, container, video/audio codecs and download speed without ffmpeg. This version introduced codec/container cautions for HEVC, fMP4 and DASH; actual support depends on the receiver and image.

A probe temporarily uses one provider connection. It may interrupt IPTV on a single-connection subscription, so it runs only when requested, one channel at a time.

## Installation and upgrade

The original package is `dreaminator_0.5.0_all.ipk`. Installing over an earlier version retains playlists and lists. Accept the GUI restart; see the [English Windows guide](docs/Dreaminator-install-Windows.pdf) and the version warning above.

## Verification at release time

- Synthetic TS/HLS streams, redirects, HEVC, HTTP 403, DASH and connection failure; Xtream/Stalker date formats and expiry warnings.
- Same package in the virtual Dreambox: Stalker live channel over HLS/MPEG-TS, H.264/AAC, demo subscription status and Python 3/2.7 plugin.
- Not yet run on the real DM520 at release time; follow-up in the [roadmap](https://github.com/4gray/dreaminator/blob/main/ROADMAP.md).

## Files

| File | SHA-256 |
| --- | --- |
| `dreaminator_0.5.0_all.ipk` | `bc86750402df1251cce9af100fd7b58b3a3fe42f0ab2df2c1c737e4b30334ed5` |

[Historical roadmap](ROADMAP.md) · [Current project documentation](https://github.com/4gray/dreaminator)
