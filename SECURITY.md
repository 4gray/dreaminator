# Security

Security fixes target the current development version. Version 0.11.2 is still a release draft; see the [installation guide](docs/installation.md) for the current build and download status. Older versions are not maintained separately.

Dreaminator is intended for a trusted home network. Its HTTP interface has no login; do not expose port 8088 to the internet. Provider credentials and playlists are stored in `/etc/dreaminator/` on the receiver. The service does not transcode streams.

Use **Report a vulnerability** on the repository's [Security page](https://github.com/4gray/dreaminator/security) when private reporting is enabled. If that option is unavailable, ask the maintainer for a private contact channel without disclosing exploit details. Include the version, receiver/image, reproduction steps and impact in the private report. Do not post provider credentials, API keys, MAC addresses or credential-bearing stream URLs in public issues, screenshots or logs.

Receiver firmware, Enigma2 players, OpenWebif and EPGImport are maintained by their respective projects. Include their versions when they affect a report.
