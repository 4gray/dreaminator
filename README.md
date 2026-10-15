# Dreaminator 0.3.0

A native TV plugin shows the tablet address, QR code and Dreaminator status in the receiver menu.

> Historical release. This package predates the DM520 startup fixes and current TV catalog. Follow the [current installation guide](https://github.com/4gray/dreaminator/blob/main/docs/installation.md) when choosing a package. The attached English Windows guide describes current setup guidance, not a rebuilt historical binary.

## New

- **Menu → Plugins → Dreaminator**, also available through extensions: receiver address and **QR code**, playlist/update status, selected channels, pending changes, EPG and errors.
- **Green:** Send to TV; **yellow:** refresh playlists; **blue:** restart the service.
- The tablet address also appears in web settings.
- Python 2.7 (OpenHDF 6.x) and Python 3 plugin support. Network requests run in the background without blocking Enigma2.

Includes 0.2.0 features: M3U URL/file, Xtream live TV, custom lists, EPGImport with multiple sources and `tvg-shift`, overnight refresh.

## Installation and upgrade

The original package is `dreaminator_0.3.0_all.ipk`. Install by USB, `opkg install` or source deployment. Installing over 0.2.0 retains playlists and lists. **Restart the GUI** after installation so the plugin appears. See the [English Windows guide](docs/Dreaminator-install-Windows.pdf) and the version warning above.

## Verification at release time

- Tests and virtual Dreambox using the same package under MIPS32 emulation.
- `make plugin-test`: Enigma2 stubs, Python 3.13/2.7, buttons, QR loading and service-unavailable handling; screen preview attached.
- Not yet run on the real DM520 at release time; follow-up in the [roadmap](https://github.com/4gray/dreaminator/blob/main/ROADMAP.md).

## Files

| File | SHA-256 |
| --- | --- |
| `dreaminator_0.3.0_all.ipk` | `621661f5e2a9978b4d3ade6c1eb81facc1367296ae3300981a33b4c35dd62901` |

[Historical roadmap](ROADMAP.md) · [Current project documentation](https://github.com/4gray/dreaminator)
