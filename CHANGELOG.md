# Changelog

All notable changes to Dreaminator are documented in this file.

## [0.12.0] - 2026-11-17

### Added
- **Multi-language Web App (15 languages)**: Full localization of all views (Channels, Lists/Bouquets, Receiver Bouquets, Sources, Settings, modals, toasts, and EPG/Picon/TMDB status messages) in `web/static/i18n.js`, `web/static/app.js`, `web/static/receiver.js`, `web/static/index.html`, and `tools/fakebox/tv.html`:
  - Portuguese (`pt`), French (`fr`), Hebrew (`he`), Arabic (`ar`), Italian (`it`), German (`de`), Spanish (`es`), Polish (`pl`), English (`en`), Japanese (`ja`), Chinese (`zh`), Thai (`th`), Turkish (`tr`), Czech (`cs`), and Russian (`ru`).
- **Automatic browser language detection**: Defaults to `"auto"` using `navigator.languages` / `navigator.language`, listens for live `languagechange` events, and includes a quick language selector in the top bar and Settings.
- **Right-to-Left (RTL) layout support**: Automatic `dir="rtl"` document switching and RTL CSS rules in `web/static/style.css` for Hebrew (`he`) and Arabic (`ar`).
- **Language-aware pluralization & date formatting**: Locale-specific plural rules and date/time formatting across all 15 supported languages.
- **Web-to-TV language synchronization**:
  - Added `Settings.Language`, `NormalizeLanguage`, `SetLanguage`, and `Summary.Language` in the Go backend (`internal/app/state.go`, `internal/app/bouquets.go`, `internal/app/info.go`).
  - Added `POST /api/language` endpoint (`internal/server/server.go`) so selecting or auto-detecting a language in the Web App updates the Enigma2 TV UI automatically.
- **Multi-language Enigma2 TV UI (Python 2.7 & Python 3)**:
  - Added `plugin/Dreaminator/ui_i18n.py` with translations, duration/day/date formatters, and server-message localization across all 15 languages.
  - Wired `ui_i18n` across `plugin.py`, `ui_shell.py`, `ui_detail.py`, `ui_search.py`, `ui_guide.py`, `ui_widgets.py`, `ui_dialogs.py`, `ui_library.py`, `ui_bouquet.py`, `ui_epgkey.py`, `ui_api.py`, and `plugin/DreaminatorEPG/plugin.py`.
  - Dynamic skin rebuilding before `Screen.__init__` so screen labels adapt to the active language, plus automatic Latin keyboard initialization (`en` layout) in `ui_models.py` / `ui_search.py` for non-Russian languages.
- **Tests**:
  - Added `web/i18n.test.mjs` validating all 15 dictionaries, placeholder parity, browser language detection, RTL detection, and `receiverUI` localization.
  - Added `TestLanguageSettings` (`internal/app/info_test.go`) and `TestLanguageEndpoint` (`internal/server/receiver_test.go`).
  - Added `I18nTests` in `tools/plugin-harness/test_shell.py` covering dictionary completeness, format specifiers, helper formatting, and HD/Full-HD skin bounds (`check_bounds`) in translated screens.

### Changed
- `internal/app/bouquets.go`: `UpdateSettings` preserves an existing `Language` when omitted and clears it back to auto-detection when `"auto"` is sent.
- `web/responsive.test.mjs`: Pinned fixture language to `ru` so existing responsive layout assertions remain deterministic regardless of host browser locale.

### Packages
- **MIPS (Dreambox DM520 / OpenHDF)**: `dreaminator_0.12.0_all.ipk`
- **ARMv7 (Vu+ Uno 4K SE / openATV)**: `dreaminator_0.12.0_vuuno4kse.ipk`

## [0.11.2] - 2026-10-29

### Added
- Native TV catalog with live channels, movies/series from Stalker portals, search, favorites, recents, and optional TMDB metadata.
- Daily programme guide, M3U catch-up, channel-list archive menu entry, and optional EPG-key integration.
- Separate MIPS (`_all.ipk`) and ARMv7 (`_vuuno4kse.ipk`) packages with automated CI/CD, QEMU smoke tests, and SHA-256 checksums.
