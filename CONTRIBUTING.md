# Contributing

Start with the [development guide](docs/development.md) and [roadmap](ROADMAP.md). Documentation is in English; application labels currently use Russian. Keep those two scopes clear when proposing translations.

For bugs, provide the receiver model, image/build, Python version, Dreaminator version, package filename and reproduction steps. State whether results come from real hardware, the simulator or the Python harness. Redact credentials and stream URLs. For security issues, follow [SECURITY.md](SECURITY.md).

Before submitting a change:

```sh
make test
make plugin-unit
make icons ui-assets   # if artwork or its generator changed; commit the outputs
make ipk ipk-vuuno4kse
python3 tools/check_packages.py
```

Native plugin and playback changes also need `make sim` and `make plugin-test`. The simulator checks the MIPS executable; use the appropriate real receiver for hardware/player claims. Keep unrelated changes out of a PR, describe the resulting behavior and list tests actually run. Repository-owner pushes follow the local Codex/Greptile review policy; CI confirms the committed change.

The project is MIT licensed. Preserve third-party notices and the bundled Geist font license.
