<p align="center">
  <img src="web/static/icon.svg" width="104" height="104" alt="Dreaminator app icon">
</p>

<h1 align="center">Dreaminator</h1>

<p align="center">
  <strong>Your IPTV, at home on your Dreambox.</strong><br>
  Organize channels from your tablet. Browse and watch with your remote.
</p>

<p align="center">
  <a href="https://github.com/4gray/dreaminator/releases"><img src="https://img.shields.io/github/v/release/4gray/dreaminator?style=flat-square&amp;color=6374ee" alt="Latest GitHub release"></a>
  <img src="https://img.shields.io/badge/source_build-0.11.0--r3-6374ee?style=flat-square" alt="Current source build 0.11.0-r3">
  <img src="https://img.shields.io/badge/platform-Enigma2-38445b?style=flat-square" alt="Platform: Enigma2">
  <img src="https://img.shields.io/badge/tested_on-Dreambox_DM520-38445b?style=flat-square" alt="Tested on Dreambox DM520">
</p>

<p align="center">
  <a href="docs/installation.md">Installation</a> ·
  <a href="docs/features.md">Features &amp; remote control</a> ·
  <a href="docs/development.md">Development</a> ·
  <a href="ROADMAP.md">Roadmap</a>
</p>

---

Dreaminator brings **M3U, Xtream Codes and Stalker/Ministra** to an Enigma2 receiver. Install one package, open the web interface on your tablet, and build the channel lists you want. Send them to the TV as ordinary bouquets alongside your satellite channels.

On the TV, a native full-screen catalog gives you live channels, programme guides, favorites and recent items. Stalker portals also provide movies and series, with optional TMDB descriptions and artwork. No browser engine runs on the receiver.

## A better way to manage your TV

- **Set up from the sofa.** Add a playlist URL, upload M3U, or connect a provider account. Star channels and drag lists into order on a tablet, phone or computer.
- **Keep the familiar channel list.** Send selected channels to ordinary Enigma2 bouquets. Tune a channel from your tablet, or edit existing receiver bouquets with drafts, copies and undo.
- **Browse with your remote.** Native poster grids, live TV, search, favorites and recents. Return from playback to the same card and the channel you were watching before.
- **Give channels their guide and logo.** XMLTV filtering, EPGImport integration and automatically generated picons. Play available M3U catch-up programmes from the guide.
- **Let the receiver do the routine work.** Overnight playlist/guide refresh, subscription expiry notices and on-demand stream checks.

[Explore all features and remote controls →](docs/features.md)

## Compatibility

| Component | Current support |
| --- | --- |
| **Tested receiver** | **Dreambox DM520** |
| **Tested image** | **OpenHDF 6.4 build 137**, Linux 3.4, Python 2.7.15 |
| **Recommended Dreaminator build for this setup** | **0.11.0-r3**, built from current source with Go 1.26.8+ |
| TV layout | Full HD 1920×1080 tested; HD 1280×720 layout tested in the harness, hardware verification pending |
| Other MIPS Enigma2 receivers/images | Unverified; compatibility depends on image, Python, player and plugins |
| Vu+ Uno 4K SE / openATV 7.5 | Experimental ARMv7 package via `make ipk-vuuno4kse`; hardware verification pending |
| Other ARM/ARM64 receivers and stock DreamOS | Unverified; neither package certifies these configurations |
| Browser | Tablet, phone or desktop browser on the receiver's home network |

The default `_all.ipk` contains a **MIPS little-endian, soft-float** executable. The separate `_vuuno4kse.ipk` contains a **32-bit ARMv7** executable and declares the receiver's machine architecture. Both builds share the same Python plugin and HD/FHD assets. Python 2.7 and 3 are covered by the plugin harness; this does not certify every image. OpenWebif is required for receiver controls; EPGImport is needed for the native channel-list/infobar guide.

> **Choose the right version.** The historical GitHub release is **0.6.0**. It predates the DM520 startup fixes, TV catalog and catch-up. For the tested DM520 setup, build **0.11.0-r3** from source. The tested OpenHDF version is a compatibility baseline; you do not need to reflash a working receiver merely to match it.

Playback depends on the receiver, image and provider. The DM520's [manufacturer specification](https://dreambox.de/download/Dreambox_DM520_QuickGuide_122016.pdf) lists HEVC up to Full HD. See the [roadmap](ROADMAP.md) for remaining audio, seeking and physical-remote checks.

## Quick start

1. **Build a compatible package** on your computer with [Go 1.26.8+](docs/development.md#toolchain):

   ```sh
   make ipk
   ```

2. **Install** `dist/dreaminator_0.11.0-r3_all.ipk` from a FAT32 USB drive through **Menu → Setup → Software management → Install local extension**. Accept the GUI restart. For SSH installation, use `make deploy HOST=<receiver-address>`.
3. **Open** `http://<receiver-address>:8088` on your tablet, or scan the QR code in **Menu → Plugins → Dreaminator → Settings**.
4. **Add a source**, choose channels or groups, then select **Send to TV**. The TV catalog can browse your source independently of bouquet selection.

**[Full setup and troubleshooting](docs/installation.md)** · **[Windows walkthrough (PDF)](docs/Dreaminator-install-Windows.pdf)** · **[Printable HTML](docs/install-windows.html)**

For **Vu+ Uno 4K SE with openATV 7.5**, run `make ipk-vuuno4kse` and install `dist/dreaminator_0.11.0-r3_vuuno4kse.ipk` instead. See the [Vu+ test installation](docs/installation.md#vu-uno-4k-se--openatv-75-experimental). The ordinary `make ipk`, `make deploy` and simulator remain DM520/MIPS builds; the ARM build uses a separate binary path and does not replace them.

Application labels currently use **Russian**; documentation is in English. The web interface has no login and is intended for a trusted home network. Keep port 8088 private to that network.

<details>
<summary><strong>Which sources support movies and catch-up?</strong></summary>

| Source | Live TV | Movies / series | Catch-up |
| --- | --- | --- | --- |
| M3U URL / file | Yes | Planned | Supported M3U archive attributes |
| Xtream account | Yes | Planned | Planned |
| Stalker / Ministra | Yes | Yes | Planned |

M3U archive URL modes include `shift`, `append`, `default`, `flussonic` and `xc`. Provider retention and actual recording availability vary. Resume, watched flags, automatic next episode, downloads and cross-source search are planned. Dreaminator does not transcode media.

</details>

<details>
<summary><strong>How does Dreaminator fit into Enigma2?</strong></summary>

A small Go service runs on the receiver and serves the tablet interface and JSON API. A Python plugin renders native TV screens and hands streams to Enigma2's player. OpenWebif handles bouquet reloads and tuning; EPGImport loads filtered programmes. Stalker streams use fresh resolved links, and media goes directly to the player.

Settings and caches live in `/etc/dreaminator/`. The separate receiver-bouquet editor saves only when explicitly requested. [Architecture, file paths and simulator →](docs/development.md)

</details>

## Build, test and contribute

```sh
make dev          # local web interface on :8088
make test         # Go vet + Go tests + Node regression tests
make sim          # virtual MIPS Dreambox in Docker
make plugin-test  # Python 2.7/3 plugin and simulator integration
```

Bug reports are most useful with the receiver model, image/build, Dreaminator version and steps to reproduce. Redact provider credentials, MAC addresses, stream URLs and API keys. See the [development guide](docs/development.md) and [roadmap](ROADMAP.md) for implementation details and work ahead.

Geist fonts are bundled under the [SIL Open Font License](plugin/Dreaminator/fonts/OFL.txt). Optional metadata comes from TMDB.

*This product uses the TMDB API but is not endorsed or certified by TMDB.*
