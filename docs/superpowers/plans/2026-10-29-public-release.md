# Public Release 0.11.2 Implementation Plan

**Goal:** Publish current Dreaminator packages and make the private repository ready for its owner to open publicly.

**Architecture:** Keep the shared Go service and Python plugin, with explicit MIPS DM520 and ARMv7 Vu+ packages. Reuse one CI workflow for pushes, PRs and release tags; publish only after its checks pass. Preserve unfinished PR work and leave repository visibility unchanged.

**Tech stack:** Go 1.26.8+, Node.js, Python 2.7/3, Docker/QEMU, GitHub Actions.

- [x] Back up uncommitted branding outside Git, fast-forward to origin/main and resolve the README against current source versions.
- [x] Confirm license and Vu+ hardware status with the owner: MIT and tested Uno 4K SE/openATV 7.5.
- [x] Integrate the approved wordmark and mark; keep rejected concept images in the local backup, outside the published repository.
- [x] Set Makefile release version to 0.11.2; update README, installation guide, printable guide and release notes with explicit package choices.
- [x] Add LICENSE, THIRD_PARTY_NOTICES.md, SECURITY.md, CONTRIBUTING.md and a bug-report template.
- [x] Add SHA-pinned CI for make test, all Python harness tests, MIPS simulation, ARM QEMU smoke, asset regeneration and both package builds. Add a tag-triggered release workflow that reuses CI and uploads IPKs, the PDF and SHA256SUMS.
- [x] Run make icons ui-assets, make test, make sim, make plugin-test and make ipk-vuuno4kse locally. Inspect both IPKs and execute the ARM binary through QEMU. Render and inspect the refreshed installation PDF.
- [x] Scan published Git refs and tracked files for secrets; run dependency vulnerability checks and actionlint. Review and narrowly document any synthetic test-key findings.
- [ ] Commit final files, run local Codex review on that commit, fix findings and re-review. Greptile is unavailable because this repository is not connected; the owner previously waived it for this repository.
- [ ] Push main once, wait for CI, tag v0.11.2 and verify the release workflow, attached checksums, package versions and repository privacy.

Validation before review: Go vet/tests and Node regressions passed on Go 1.26.10; 104 Python unit tests passed on each of Python 3.14 and Docker Python 2.7, with simulator API integration on both. Both IPKs passed CPU/version/plugin-asset/license comparisons and QEMU version smoke checks. Gitleaks (published refs and working files), govulncheck and actionlint passed. Browser light/dark branding and all four PDF pages were inspected. GitHub private vulnerability reporting cannot be enabled while this repository is private (API 404); SECURITY.md includes a private-contact fallback.
