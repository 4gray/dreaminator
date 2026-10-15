# Public repository documentation design

## Purpose

Prepare Dreaminator's GitHub documentation for an English-speaking audience while keeping repository visibility private until the owner changes it. Application localization is a separate project.

## Information structure

The README uses the existing app icon and restrained version/platform badges, followed by the product purpose, key features, an evidence-based compatibility table and quick start. Long feature/support details use linked guides and collapsible sections.

- `docs/installation.md`: tested versions, install paths, first use, EPGImport, receiver bouquets, optional TMDB, troubleshooting and removal.
- `docs/features.md`: source matrix, features, remote bindings and current limits.
- `docs/development.md`: toolchain, commands, simulator, architecture, receiver files and contribution guidance.
- `ROADMAP.md`: English milestone/status record preserving the distinction between implementation, simulator results and hardware validation.
- `docs/install-windows.html` and `docs/Dreaminator-install-Windows.pdf`: accessible English walkthrough with generic user instructions.
- Existing engineering plans/specifications: English, with Russian prose and display-label quotations translated where present.

## Accuracy requirements

Recommend current source build 0.11.0-r3 for the tested DM520/OpenHDF 6.4 build 137 configuration. Mark GitHub 0.6.0 as historical and lacking hardware fixes. Explain that the all-architecture package actually embeds a MIPS executable. Do not certify untested devices, images, codecs or physical-remote behavior. State that application labels currently use Russian.

## GitHub and history

Translate the five historical release descriptions without rebuilding their binaries. Replace Russian installation PDF attachments with an English equivalent, keeping originals in a local backup. Inventory branches/tags/open PRs before rewriting Git references. Keep a verified Git bundle and release metadata/assets outside the repository. The owner approved a clean root commit on 2026-10-15. Preserve the unmerged PR as a child commit of the new root; preserve each historical tag as a parentless source snapshot with English documentation. Remove only remote branches already merged into the original main. Publish all ref changes atomically with explicit force-with-lease guards. No repository visibility change, PR merge or license choice is implied.

## Verification

Check English-only documentation, relative links, version consistency, PDF page rendering and text extraction. Run existing checks for changed build/documentation commands. Review the committed changes locally using Codex; run Greptile if repository access is configured, otherwise report its precise limitation. Verify remote refs and release contents after authorized publication.
