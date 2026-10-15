# Dreaminator 0.6.0

Channel logos on TV: Dreaminator turns playlist artwork into Enigma2 picons for the channel list and infobar.

> Historical release. This package predates the DM520 startup fixes and current TV catalog. For the tested receiver configuration, follow the [current installation guide](https://github.com/4gray/dreaminator/blob/main/docs/installation.md). The attached English Windows guide describes current setup guidance; it does not change this historical binary.

## New

- **Picons for selected channels:** M3U `tvg-logo`, Xtream and Stalker artwork fitted into standard 220×132 picons. PNG, JPEG, GIF and WebP supported.
- **Dark logos** receive a light backing for visibility on dark skins; colored logos keep transparency.
- **Automatic directory selection:** use the directory Enigma2 already searches, otherwise `/usr/share/enigma2/picon`. A manifest ensures only Dreaminator's own files are changed or removed.
- Logos refresh after **Send to TV** and overnight; unchanged artwork is not downloaded again.
- Settings show logo counts, missing-logo reasons, refresh, enable/disable and the selected directory.

## Installation and upgrade

The original package is `dreaminator_0.6.0_all.ipk`. Installing over an earlier version retains playlists and lists. Accept the GUI restart. Logos appear after the first **Send to TV**. See the [English Windows guide](docs/Dreaminator-install-Windows.pdf) and the version warning above.

## Verification at release time

- Tests: picon names, fitting/backing, directory selection, manifest synchronization, repeated downloads, removal, preservation of unrelated picons and disabling.
- Virtual Dreambox using the same package: real M3U artwork and demo Stalker logos, picons for 9 of 11 channels (two had no logo); simulated TV finds them using Enigma2 rules. Preview attached.
- The package had not yet run on the real DM520 at release time. Hardware follow-up is recorded in the [roadmap](https://github.com/4gray/dreaminator/blob/main/ROADMAP.md).

## Files

| File | SHA-256 |
| --- | --- |
| `dreaminator_0.6.0_all.ipk` | `3d3b3f5246149c73a536013170443b8ac0e75a476c9497d0bc7d2258b9a7452f` |

[Historical roadmap](ROADMAP.md) · [Current project documentation](https://github.com/4gray/dreaminator)
