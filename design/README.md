# Abacad design tokens

`tokens.json` is the single source of truth for Abacad's visual language —
colors, spacing, radii, and type sizes — shared by all three surfaces:

| Surface | Generated file | Consumed as |
|---|---|---|
| Web dashboard (`server/frontend`) | `src/tokens.css` | CSS custom properties (`--brand`, `--space-md`, …) |
| Android probe (`android/`) | `probe/Theme.kt` | `Theme.BRAND` ARGB ints, `Theme.SPACE_MD` dp, `Theme.TEXT_MD` sp |
| macOS agent (`macos/`) | `AbacadAgent/Theme.swift` | `Theme.brand` SwiftUI `Color`, `Theme.spaceMd` CGFloat |

To change the palette or metrics, edit `tokens.json` and regenerate:

    make tokens        # or: node design/generate.mjs

The generated files are committed (the Android and macOS builds must not
depend on Node), so re-run the generator and commit them together with the
JSON change. Each generated file carries a GENERATED header — never edit
them by hand.

## Semantics

- **Palette** is dark-first; the dashboard is dark-only today and both
  clients follow it. `canvas → sidebar → surface → surface-raised →
  surface-hover` is the elevation ramp; `ink / ink-muted / ink-subtle` is
  the text ramp.
- **Status colors** are the connection-state language used everywhere:
  `success` = connected, `warning` = connecting/reconnecting,
  `danger` = disconnected. `*-strong` variants are for saturated fills on
  light surfaces; the base tones are tuned for dark backgrounds.
- **`brand`** (green) is the product accent — same hue as `success` by
  design: "connected" *is* the brand promise. The launcher icon
  (`assets/icon.svg`) keeps its own blue/coral mark; that is the logo,
  not the UI palette.
- **Native chrome stays native.** The macOS menu-bar panel keeps system
  materials and fonts, and Android keeps DeviceDefault (dark) widgets;
  tokens supply the shared semantics (status colors, spacing, canvas) on
  top, not a full custom widget skin.
