# Vu+ Uno 4K SE build implementation plan

**Goal:** Provide a testable package for Vu+ Uno 4K SE / openATV 7.5 while preserving the default Dreambox DM520 build.

**Architecture:** Share the Go service, web assets, Python plugin and init script. Keep `make box`, `make ipk`, `make deploy` and the MIPS simulator unchanged. Add explicit ARMv7 targets with separate binary and package paths. Use the receiver's machine package architecture, `vuuno4kse`, instead of claiming the ARM binary is universal.

**Tech stack:** Go 1.26.8, GNU Make, opkg IPK, Python 2.7/3, Docker and QEMU.

## Scope and acceptance

- Default build remains Linux MIPS little-endian / soft-float, including its existing output paths and version.
- `make box-vuuno4kse` builds a static Linux ARMv7 executable in `dist/vuuno4kse/dreaminator`.
- `make ipk-vuuno4kse` writes `dist/dreaminator_0.11.0-r3_vuuno4kse.ipk` with `Architecture: vuuno4kse`.
- The packager rejects a wrong CPU, byte order, word size, dynamic executable or unsupported package architecture before writing output.
- Both packages contain the same plugin, fonts, HD/FHD assets and startup script.
- Hardware playback and the physical remote remain unverified until installation on the receiver.

## Tasks

- [x] Add failing Go tests in `tools/mkipk/main_test.go` for supported binaries and rejected architecture mismatches, malformed ELF and dynamic linking. Add Make dry-run checks for separate outputs and preservation of the default build.
- [x] Run `go test ./tools/mkipk` and confirm failures come from the missing validation and ARM targets.
- [x] Add `-arch` (default `all`, preserving the existing MIPS package) and ELF validation in `tools/mkipk/main.go`. Write the selected architecture into package control metadata.
- [x] Add `box-vuuno4kse` and `ipk-vuuno4kse` to `Makefile`, using `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0`. Keep the default targets unchanged.
- [x] Update `README.md`, `docs/development.md` and `docs/installation.md` with the explicit build command, filenames and Vu+ installation/testing instructions.
- [x] Run Go vet/tests, browser tests and Python 2.7/3 plugin checks.
- [x] Build both actual IPKs. Extract them, inspect metadata, ELF architecture, static linking and Go build settings, and compare all shared payload files. Rebuild the default package after ARM and confirm it still contains MIPS.
- [x] Start both extracted services under QEMU against the fake receiver, check HTTP health/settings, and run catalog integration against each in isolation. Record limits of emulation in the handoff.
- [x] Review the diff, fix findings, and deliver both local package links with installation instructions. No push or receiver deployment is requested.
