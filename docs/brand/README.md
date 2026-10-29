# Modular Stream identity

The approved Modular Stream concept is used in the README, web app and Enigma2 plugin.

- Accent: cyan `#38bdf8`; graphite `#171b22`; off-white `#f4f6f8`.
- The segmented stem represents a digital stream; the triangle represents playback.
- `dreaminator-wordmark.png` is the README banner, generated with built-in ImageGen using the approved Modular Stream concept as its edit reference. Earlier explorations are archived outside the release repository.
- The compact mark was redrawn as deterministic geometry in `internal/brand/brand.go`. It deliberately adds cyan to the three stream segments for visibility at small sizes.
- `make icons` regenerates the web SVG, 192/512 px PNGs and the plugin menu PNG. The latter remains 100×40 with a centred 40×40 mark and transparent side gutters.
- `make ui-assets` regenerates the catalog, guide and About marks at 48 px (Full HD) and 32 px (HD), using the same renderer.
- The README banner has a graphite background for consistent contrast in light and dark GitHub themes. The app icons use the same graphite tile with transparent rounded corners.

## Exact ImageGen edit prompt

Use case: precise-object-edit / logo-brand. Turn the supplied approved Dreaminator Modular Stream brand concept into ONE final production README wordmark banner. Preserve the exact distinctive engineering semi-monospaced letterforms and spelling 'Dreaminator', large white typography, first D with inset play triangle, and trailing square. Important change: replace the desaturated steel-blue accent with lively clear cyan-blue #38BDF8 on the play triangle and trailing square. It should feel like crisp live digital television, optimistic and confident. Flat uniform graphite #171B22 background, offwhite #F4F6F8 letters, no gradient, no texture, no glow, no shadow. Composition: ONLY the large wordmark with play inside initial D and trailing cyan square, horizontally centered on a tightly composed wide banner with modest even margins (approximately 8% left/right and 22% above/below). Remove the '03 / MODULAR STREAM' heading and remove the separate small icon at bottom completely. No labels, no tagline, no other objects, no mockup. Keep the type exactly recognizable from this approved reference, do not replace with generic sans serif. Output one horizontal approximately 4:1 banner, crisp flat artwork.
