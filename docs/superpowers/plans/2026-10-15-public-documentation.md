# Public documentation implementation plan

**Goal:** Prepare all repository documentation and release text in English, with an approachable README and recoverable history changes.

**Architecture:** Keep the existing app icon, split long README material into focused guides, and use existing HTML-to-PDF tooling. Git history and release metadata are managed separately from runtime code.

**Tech stack:** Git, GitHub CLI, Markdown, HTML, Chrome PDF printing, existing Go/Node verification tools.

- [x] Inventory tracked documentation, remote branches/tags, releases and open pull requests.
- [x] Save and verify a complete local Git bundle, remote ref inventory and original release metadata outside the repository.
- [x] Draft the README with icon, badges, compatibility table, quick start and collapsible detail.
- [x] Rewrite installation/features/development guides and roadmap in English; translate remaining specification labels.
- [x] Rename the Windows PDF to an English filename, update `make docs`, regenerate and inspect every PDF page.
- [x] Translate historical release notes and replace installation guide attachments after backing them up.
- [x] Validate documentation language, local links and current version guidance; run relevant existing checks.
- [x] Commit and perform local review, resolve findings and re-review the final changes.
- Publish the approved clean-root history while preserving unmerged code and release source snapshots; use explicit force-with-lease ref updates.
- Verify remote branch/tag/release state and report the backup location and remaining publication decisions. Keep the final verification report alongside the local backup, outside the published source tree.
