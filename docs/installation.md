# Installation and setup

[Back to the README](../README.md) · [Features and remote control](features.md) · [Development](development.md)

## Choose a receiver and version

The tested configuration is **Dreambox DM520 with OpenHDF 6.4 build 137**, Linux 3.4, Python 2.7.15 and the FullHDLine 1920×1080 skin. This is a compatibility baseline, not a recommendation to replace a working receiver image.

Use the current source build, **0.11.0-r3**, for the DM520. Build it with `make ipk` using Go 1.26.8 or newer. The older GitHub release **0.6.0 does not include the DM520 startup fixes, TV catalog or catch-up**. Check the version before installing a downloaded package.

The default package contains a **Linux MIPS little-endian, soft-float** executable. Its `_all.ipk` filename and `Architecture: all` package field do not make it a universal binary. Other MIPS Enigma2 receivers and images are unverified. A separate experimental ARMv7 package targets **Vu+ Uno 4K SE / openATV 7.5**; see below. Other ARM/ARM64 receivers, stock DreamOS and other package managers are unverified.

The plugin supports Python 2.7 and Python 3 in the test harness. Python 3 tests do not certify a particular receiver or image. Full HD is tested on hardware; the 1280×720 layout is covered by layout tests and still needs hardware verification.

## Requirements

- A compatible receiver connected to your home network, with `opkg` and OpenWebif.
- A browser on a tablet, phone or computer on the same network.
- Your provider's M3U playlist, Xtream credentials, or Stalker/Ministra portal credentials.
- EPGImport if you want programme information in the native Enigma2 channel list and infobar.
- A FAT32 USB drive for installation from the receiver menus, or SSH access for network installation.

Dreaminator runs a local HTTP service on port **8088** without a login. Keep it on a trusted home network; do not forward that port to the internet. Provider credentials and uploaded playlists are stored on the receiver in `/etc/dreaminator/`. Redact credentials, MAC addresses and stream URLs when sharing logs or screenshots.

## Install the package

### USB drive

1. Copy `dreaminator_<version>_all.ipk` to the root of a FAT32 USB drive.
2. Connect it to the receiver.
3. Open **Menu → Setup → Software management → Install local extension**. Some images have an extra **Software** submenu.
4. Select the USB drive and the Dreaminator package, then confirm installation.
5. Accept the **GUI restart** so Enigma2 discovers the plugin.

For a more detailed Windows walkthrough, see the [printable HTML guide](install-windows.html) or [PDF guide](Dreaminator-install-Windows.pdf). Menu names vary between images. Dreaminator's application interface currently uses Russian labels; the English names in these guides explain their meaning.

### SSH from a source checkout

With Go 1.26.8+ installed on your computer:

```sh
make deploy HOST=192.168.1.45
```

Replace the example address with your receiver's IP or hostname. This builds the current package, copies it over SSH and installs it. The default hostname is `dm520.fritz.box`. SSH compatibility options for the old Dropbear server are configured in the Makefile.

### Manual installation

Build with `make ipk`, or obtain a package whose version and architecture match your receiver. Copy it to `/tmp` using your preferred transfer tool, then run on the receiver:

```sh
opkg install /tmp/dreaminator_0.11.0-r3_all.ipk
```

The service starts immediately and on subsequent boots. Restart the Enigma2 GUI to load the plugin. Installing over an earlier version retains playlists and lists; back up `/etc/dreaminator/` and `/etc/enigma2/` before upgrading.

### Vu+ Uno 4K SE / openATV 7.5 (experimental)

This target is for the **32-bit ARMv7** Uno 4K SE (`vuuno4kse`). The reference receiver reports openATV 7.5 dated 29 December 2024, with the MetrixHD 1280×720 skin. HD assets are included, but GUI, physical-remote and playback behavior still require testing on that receiver. You do not need to reflash openATV to try the package.

Build on your computer with Go 1.26.8 or newer:

```sh
make ipk-vuuno4kse
```

Use **`dist/dreaminator_0.11.0-r3_vuuno4kse.ipk`**. The `_all.ipk` file is the Dreambox MIPS build and cannot run on this Vu+. ARM and MIPS binaries are built into separate paths, and both IPKs can coexist in `dist/`. The ordinary `make deploy` still targets DM520/MIPS, even if you change `HOST`; it does not select the ARM package.

For USB installation, copy the `_vuuno4kse.ipk` file to a FAT32 drive and follow the local-extension steps above. For SSH installation, copy that file to the receiver's `/tmp/` and run:

```sh
opkg install /tmp/dreaminator_0.11.0-r3_vuuno4kse.ipk
```

Restart the **Enigma2 GUI** from the receiver's menu. Open **Plugins → Dreaminator**, then use the shown address on your phone/tablet (`http://<receiver-address>:8088`). OpenWebif is required; install EPGImport if you want the guide in native bouquets/infobar. Before updating an existing installation, back up `/etc/dreaminator/` and `/etc/enigma2/` as described above.

For the first hardware test, check:

1. The web interface opens and reports version `0.11.0-r3`.
2. The TV catalog fits the 1280×720 screen; arrows, OK, Back and color keys work.
3. A known live channel plays, and Back returns to the catalog correctly. If your provider has VOD, also test opening a movie/episode and returning from it.
4. **Send to TV** creates the expected bouquet; test EPG import if EPGImport is installed.
5. After a normal receiver reboot, the web service starts again.

If the package is rejected for architecture, collect `opkg print-architecture` and `uname -m`; do not bypass the architecture check. If it installs but does not start, collect `/tmp/dreaminator.log` and the output of `/usr/bin/dreaminator -version`. If the TV interface fails, include the Enigma2 crash log when available. Redact provider credentials and URLs.

## Open Dreaminator

1. On the TV, open **Menu → Plugins → Dreaminator**. Scan the QR code in **Settings**, or enter `http://<receiver-address>:8088` in your browser.
2. Bookmark the page. On Android Chrome, **Add to Home screen** creates a convenient app shortcut.
3. Prefer a stable local hostname, such as `http://dm520.fritz.box:8088`, or reserve the receiver's IP in your router. The plugin's QR code uses the current IP.

If no sources exist, Dreaminator opens the settings/status screen first. After adding a source, the TV catalog is available without creating a bouquet.

## From a playlist to your TV channel list

1. Open **Playlists → Add** and choose M3U URL, M3U file, Xtream or Stalker.
2. For a portal, enter the address and MAC supplied by your provider. Add login, password, serial number and Device ID only if required by that provider.
3. Open **Choose groups and channels**. Use a star to add a channel to a list, or turn an entire group into a separate list.
4. Reorder lists and channels by dragging them in **My lists**.
5. Select **Send to TV**. The lists appear as ordinary Enigma2 bouquets alongside your existing channels without a GUI restart.

Adding a playlist alone does not create bouquets. The TV catalog and bouquet selection are independent. Movies and series never become live TV bouquet entries.

Dreaminator refreshes playlists and guides at **04:00** and sends pending list changes to TV. Leave the receiver powered in standby for scheduled updates. After a full power-off, it catches up when started again.

## Programme guide

Dreaminator reads XMLTV `.xml` and `.gz`, including multiple URLs in priority order. It matches `tvg-id`, channel name and `tvg-name`, and applies `tvg-shift`. Xtream and Stalker can supply provider EPG automatically when no custom EPG URL is set.

For the native receiver EPG, after your first **Send to TV**:

1. Install **EPGImport / EPG-Importer** through **Plugins → Download → Extensions** if needed.
2. Open EPG-Importer and press **blue** for **Sources**.
3. Ensure **Dreaminator** is selected, then save with **green**.
4. Enable automatic import at **05:00**, after Dreaminator's overnight refresh.
5. Run a manual import with **yellow** to populate the guide immediately.

On **openATV 7.5**, EPGImport is hidden from the Plugin Browser by default. Open **Menu → Setup → EPG → EPG-Importer** (Russian: **Меню → Настройка → ТВ-гид → Импортер EPG**). Enable **Show EPG-Importer in plugins list** and save if you want it in the Plugin Browser. Dreaminator exports full IPTV service references so this image's EPGImport validation accepts the synthetic channel IDs; the stream URLs are not fetched during EPG import.

The TV catalog also has its own guide. Selected channels use the filtered XMLTV file; other M3U channels use a compact 36-hour index; Xtream/Stalker short guides are fetched on demand and cached for 15 minutes. The catalog does not require channels to be in bouquets. Manual EPG matching and `.xz` guides are not implemented.

## Existing receiver bouquets

Open **My lists → Receiver bouquets** to view bouquets created by OpenWebif or another plugin. Rename, reorder or remove entries in a draft, then choose **Save to TV** for that bouquet. These drafts are separate from Dreaminator's **Send to TV** and overnight synchronization.

Original service references and markers are preserved. A revision check rejects stale saves after another plugin changes a file. **Undo** restores the last save while the file remains unchanged. Prefer **Create copy** when a provider plugin regularly regenerates its own bouquets.

Deleting a receiver bouquet removes it immediately from `bouquets.tv` and `/etc/enigma2/`. Recovery snapshots remain in `/etc/dreaminator/receiver-undo/` as `deleted-<file>-<hash>` and `index-<hash>`. A deleted Dreaminator list disappears from TV after **Send to TV**.

## Optional TMDB metadata

In **Settings → TMDB descriptions and posters**, enter your own [TMDB v3 API key](https://www.themoviedb.org/settings/api), choose a metadata language and test the connection. The key is stored on the receiver, masked in the interface and omitted from state responses and logs. You can disable the integration or delete the key.

TMDB supplies localized titles, descriptions, genres, ratings, cast, backdrops and episode information. Provider data remains the fallback. Playback does not depend on TMDB. Ambiguous matches can be corrected or reset in the web interface.

*This product uses the TMDB API but is not endorsed or certified by TMDB.*

## Troubleshooting

<details>
<summary>The web interface does not open</summary>

Check the receiver's current IP under **Menu → Information → System information** and confirm that your browser uses `http://<address>:8088`. Both devices must be on the same network. An address beginning with `169.254` usually means the receiver did not obtain a network address. Wait for startup, then inspect the service over SSH:

```sh
/etc/init.d/dreaminator restart
tail -n 100 /tmp/dreaminator.log
```

If a home-screen shortcut stopped working after a power outage, update it to the new address or configure an IP reservation.

</details>

<details>
<summary>The plugin is missing or a stream fails</summary>

Restart the **Enigma2 GUI** after installation. A service restart alone does not reload the plugin or its keymap.

Use the channel's stream check to inspect HTTP response time, container, codecs and download speed. A probe temporarily uses one provider connection and may interrupt viewing on a single-connection subscription. Container support does not guarantee codec support. Playback, seeking, subtitles and audio depend on the receiver image, player and provider. DM520 HEVC is specified only up to Full HD, not 4K; see the [manufacturer's quick guide](https://dreambox.de/download/Dreambox_DM520_QuickGuide_122016.pdf).

On Linux 3.4, use a package built with Go 1.26.8+; the Go 1.26.0 build crashed at startup with `futexwakeup ... returned -89` during hardware testing.

</details>

<details>
<summary>Catch-up is missing or a recording does not start</summary>

Catch-up currently works with supported M3U archive attributes. Xtream account and Stalker archive integrations are planned. A provider must actually retain the requested recording; an advertised archive is not a guarantee that every channel has one.

Dreaminator accumulates past programme titles during overnight guide refreshes. A newly installed receiver cannot reconstruct days its EPG provider no longer supplies. Some providers make the latest few minutes available only after a delay. Sliding HLS archives may not support the native player's seeking controls. See the [roadmap](../ROADMAP.md) for hardware verification still outstanding.

</details>

## Remove Dreaminator

```sh
opkg remove dreaminator
```

TV lists remain and can be removed with the remote. The original `bouquets.tv` is backed up alongside it as `bouquets.tv.dreaminator-backup`.
