# abacad design tokens

`tokens.json` is the single source of truth for abacad's visual language —
colors, spacing, radii, and type sizes — shared by all three surfaces:

| Surface | Generated file | Consumed as |
|---|---|---|
| Web dashboard (`server/frontend`) | `src/tokens.css` | CSS custom properties (`--brand`, `--space-md`, …) |
| Android app (`android/`) | `android/Theme.kt` | `Theme.BRAND` ARGB ints, `Theme.SPACE_MD` dp, `Theme.TEXT_MD` sp |
| macOS agent (`macos/`) | `abacad/Theme.swift` | `Theme.brand` SwiftUI `Color`, `Theme.spaceMd` CGFloat |

To change the palette or metrics, edit `tokens.json` and regenerate:

    make tokens        # or: node design/generate.mjs

The generated files are committed (the Android and macOS builds must not
depend on Node), so re-run the generator and commit them together with the
JSON change. Each generated file carries a GENERATED header — never edit
them by hand.

## Semantics

- **Palette** comes in a `dark` and a `light` scheme with identical token
  names; every surface defaults to **auto** (follow the system appearance).
  The dashboard resolves via `prefers-color-scheme` with a `data-theme`
  override on `<html>` (the header toggle cycles auto → light → dark),
  Android resolves via the uiMode night flag (`Theme.of(resources)` +
  DayNight system theme), and the macOS colors are appearance-dynamic
  `NSColor`s. `canvas → sidebar → surface → surface-raised → surface-hover`
  is the elevation ramp; `ink / ink-muted / ink-subtle` is the text ramp;
  `shadow / shadow-strong` are the elevation shadows (soft in light).
- **Status colors** are the connection-state language used everywhere:
  `success` = connected, `warning` = connecting/reconnecting,
  `danger` = disconnected. Each scheme carries its own contrast-tuned
  tones; `*-strong` variants are the deeper fill/hover versions within a
  scheme.
- **`brand`** (green) is the product accent — same hue as `success` by
  design: "connected" *is* the brand promise. The launcher icon
  (`assets/icon.svg`) keeps its own blue/coral mark; that is the logo,
  not the UI palette.
- **Native chrome stays native.** The macOS menu-bar panel keeps system
  materials and fonts, and Android keeps DeviceDefault (DayNight) widgets;
  tokens supply the shared semantics (status colors, spacing, canvas) on
  top, not a full custom widget skin.
