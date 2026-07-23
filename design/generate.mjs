#!/usr/bin/env node
// Generate per-platform design tokens from design/tokens.json — the single
// source of truth for abacad's visual language across the web dashboard,
// the Android app, and the macOS agent.
//
//   node design/generate.mjs
//
// Colors come in a dark and a light scheme. Every surface defaults to "auto"
// (follow the system appearance): the CSS defers to prefers-color-scheme
// unless data-theme forces a scheme, the Kotlin palette is resolved from the
// uiMode night flag, and the Swift colors are appearance-dynamic NSColors.
//
// Outputs (all committed, all marked GENERATED):
//   server/frontend/src/tokens.css                       CSS custom properties
//   android/app/src/main/java/ai/abacad/android/Theme.kt  Kotlin palettes (ARGB ints, dp/sp)
//   macos/Sources/abacad/Theme.swift                SwiftUI dynamic Colors + CGFloats
//   linux/internal/gui/theme_gen.go                 Go palettes for the gotk4/libadwaita GUI
//   windows/Theme.xaml                              WinUI 3 ResourceDictionary (ThemeDictionaries)
//
// The Jetpack Compose theme (Color/Dp/Material3 scheme) is emitted alongside the
// Android Compose migration, where the Compose deps that make it compile live.

import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const tokens = JSON.parse(readFileSync(join(root, "design", "tokens.json"), "utf8"));

const HEADER = "GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs";

const { dark, light } = tokens.color;
{
  const d = Object.keys(dark).join(",");
  const l = Object.keys(light).join(",");
  if (d !== l) throw new Error(`dark and light schemes must define the same tokens\n dark:  ${d}\n light: ${l}`);
}

// "#rrggbb" or "#rrggbbaa" → { r, g, b, a } with 0-255 channels.
function parse(hex) {
  const h = hex.replace("#", "");
  if (h.length !== 6 && h.length !== 8) throw new Error(`bad hex ${hex}`);
  return {
    r: parseInt(h.slice(0, 2), 16),
    g: parseInt(h.slice(2, 4), 16),
    b: parseInt(h.slice(4, 6), 16),
    a: h.length === 8 ? parseInt(h.slice(6, 8), 16) : 255,
  };
}

const upperSnake = (name) => name.replace(/-/g, "_").toUpperCase();
const camel = (name) => name.replace(/-(\w)/g, (_, c) => c.toUpperCase());
const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);

// --- CSS ---------------------------------------------------------------
{
  const vars = (scheme, indent) =>
    Object.entries(scheme)
      // CSS keeps the source notation; 8-digit hex is valid CSS Color 4.
      .map(([name, hex]) => (parse(hex), `${indent}--${name}: ${hex};\n`))
      .join("");

  let css = `/* ${HEADER} */\n\n:root {\n`;
  for (const [name, px] of Object.entries(tokens.space)) css += `  --space-${name}: ${px}px;\n`;
  for (const [name, px] of Object.entries(tokens.radius)) css += `  --radius-${name}: ${px}px;\n`;
  for (const [name, px] of Object.entries(tokens.font.size)) css += `  --text-${name}: ${px}px;\n`;
  css += `}\n\n`;
  css += `/* Dark scheme — the base, and forced via data-theme="dark". */\n`;
  css += `:root {\n  color-scheme: dark;\n${vars(dark, "  ")}}\n\n`;
  css += `/* Light scheme — auto (default: follow the system) … */\n`;
  css += `@media (prefers-color-scheme: light) {\n  :root:not([data-theme="dark"]) {\n    color-scheme: light;\n${vars(light, "    ")}  }\n}\n\n`;
  css += `/* … or forced via data-theme="light". */\n`;
  css += `:root[data-theme="light"] {\n  color-scheme: light;\n${vars(light, "  ")}}\n`;
  writeFileSync(join(root, "server", "frontend", "src", "tokens.css"), css);
}

// --- Kotlin ------------------------------------------------------------
{
  const argb = (hex) => {
    const { r, g, b, a } = parse(hex);
    const v = ((a << 24) | (r << 16) | (g << 8) | b) >>> 0;
    return `0x${v.toString(16).toUpperCase().padStart(8, "0")}.toInt()`;
  };
  const palette = (scheme) =>
    `    val ${scheme === dark ? "DARK" : "LIGHT"} = Palette(\n` +
    Object.entries(scheme)
      .map(([name, hex]) => `        ${upperSnake(name)} = ${argb(hex)},\n`)
      .join("") +
    `    )\n`;

  let kt = `package ai.abacad.android\n\nimport android.content.res.Configuration\nimport android.content.res.Resources\n\n// ${HEADER}\n\n`;
  kt += `/**\n * abacad design tokens. Colors are ARGB ints (pass straight to setTextColor /\n * setBackgroundColor) and come as a dark and a light [Palette] — call\n * [Theme.of] to get the one matching the current system appearance (auto\n * dark/light is the product default on every surface). SPACE_* are dp,\n * TEXT_* are sp — multiply dp values by displayMetrics.density before use.\n */\nobject Theme {\n`;
  kt += `    class Palette internal constructor(\n`;
  for (const name of Object.keys(dark)) kt += `        val ${upperSnake(name)}: Int,\n`;
  kt += `    )\n\n`;
  kt += palette(dark) + `\n` + palette(light) + `\n`;
  kt += `    /** The palette for the current system appearance (uiMode night flag). */\n`;
  kt += `    fun of(resources: Resources): Palette {\n        val night = resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK\n        return if (night == Configuration.UI_MODE_NIGHT_NO) LIGHT else DARK\n    }\n\n`;
  for (const [name, px] of Object.entries(tokens.space)) kt += `    const val SPACE_${upperSnake(name)} = ${px} // dp\n`;
  for (const [name, px] of Object.entries(tokens.radius)) kt += `    const val RADIUS_${upperSnake(name)} = ${px} // dp\n`;
  for (const [name, px] of Object.entries(tokens.font.size)) kt += `    const val TEXT_${upperSnake(name)} = ${px}f // sp\n`;
  kt += `}\n`;
  writeFileSync(join(root, "android", "app", "src", "main", "java", "ai", "abacad", "android", "Theme.kt"), kt);
}

// --- Swift -------------------------------------------------------------
{
  const f = (v) => (v / 255).toFixed(4);
  const quad = (hex) => {
    const { r, g, b, a } = parse(hex);
    return `(${f(r)}, ${f(g)}, ${f(b)}, ${f(a)})`;
  };

  let sw = `import AppKit\nimport SwiftUI\n\n// ${HEADER}\n\n`;
  sw += `/// abacad design tokens. The menu-bar panel keeps native macOS materials for\n/// its chrome; these tokens supply the shared semantic colors (status, brand)\n/// and metrics so the panel reads as the same product as the dashboard. Each\n/// color is appearance-dynamic — it resolves to the dark or light variant as\n/// the system (or panel) appearance changes, so auto dark/light needs no code.\nenum Theme {\n`;
  sw += `    private typealias RGBA = (r: CGFloat, g: CGFloat, b: CGFloat, a: CGFloat)\n\n`;
  sw += `    private static func dynamic(dark: RGBA, light: RGBA) -> Color {\n`;
  sw += `        Color(nsColor: NSColor(name: nil) { appearance in\n`;
  sw += `            let c = appearance.bestMatch(from: [.darkAqua, .aqua]) == .aqua ? light : dark\n`;
  sw += `            return NSColor(srgbRed: c.r, green: c.g, blue: c.b, alpha: c.a)\n`;
  sw += `        })\n    }\n\n`;
  for (const name of Object.keys(dark)) {
    sw += `    static let ${camel(name)} = dynamic(dark: ${quad(dark[name])}, light: ${quad(light[name])})\n`;
  }
  sw += `\n`;
  const metric = (prefix, name, px) =>
    `    static let ${prefix}${upperSnake(name).charAt(0) + camel(name).slice(1).toLowerCase()}: CGFloat = ${px}\n`;
  for (const [name, px] of Object.entries(tokens.space)) sw += metric("space", name, px);
  for (const [name, px] of Object.entries(tokens.radius)) sw += metric("radius", name, px);
  for (const [name, px] of Object.entries(tokens.font.size)) sw += metric("text", name, px);
  sw += `}\n`;
  writeFileSync(join(root, "macos", "Sources", "abacad", "Theme.swift"), sw);
}

// --- Linux (GTK4 / libadwaita, consumed by the gotk4 GUI) --------------
// A Go source file in the GUI package: dark + light palettes as CSS-ready hex
// strings plus dp/px metrics. The window keeps native Adwaita chrome; these
// feed a CssProvider string and direct gdk.RGBA where our status/brand colors
// override the theme. Mirrors Theme.kt/Theme.swift — constants in code, so the
// cgo-free headless build never depends on this (the file isn't imported by
// cmd/abacad; it only compiles into the GUI build).
{
  const goField = (k) => cap(camel(k));
  // gofmt aligns struct fields and keyed-literal values into columns; emit that
  // shape directly so `node generate.mjs` output is already gofmt-clean.
  const fieldW = Math.max(...Object.keys(dark).map((k) => goField(k).length));
  const goPalette = (scheme, name) =>
    `var ${name} = Palette{\n` +
    Object.entries(scheme)
      .map(([k, hex]) => {
        parse(hex);
        const key = `${goField(k)}:`;
        return `\t${key}${" ".repeat(fieldW + 2 - key.length)}${JSON.stringify(hex)},\n`;
      })
      .join("") +
    `}\n`;

  let go = `package gui\n\n// ${HEADER}\n\n`;
  go += `// Palette is one appearance's abacad tokens as CSS-ready hex strings\n// ("#rrggbb" or "#rrggbbaa"). Feed them to a GTK CssProvider or gdk.RGBA.Parse.\n`;
  go += `type Palette struct {\n`;
  for (const k of Object.keys(dark)) go += `\t${goField(k)}${" ".repeat(fieldW + 1 - goField(k).length)}string\n`;
  go += `}\n\n`;
  go += goPalette(dark, "Dark") + `\n` + goPalette(light, "Light") + `\n`;
  go += `// Of returns the palette for the active appearance; pass\n// adw.StyleManager.Dark() (true = dark).\n`;
  go += `func Of(dark bool) Palette {\n\tif dark {\n\t\treturn Dark\n\t}\n\treturn Light\n}\n\n`;
  go += `// Metrics (px). GTK works in device-independent px; scale by the surface\n// factor if you target hidpi manually (GTK usually handles it).\n`;
  for (const [n, px] of Object.entries(tokens.space)) go += `const Space${cap(n)} = ${px}\n`;
  for (const [n, px] of Object.entries(tokens.radius)) go += `const Radius${cap(n)} = ${px}\n`;
  for (const [n, px] of Object.entries(tokens.font.size)) go += `const Text${cap(n)} = ${px}\n`;
  const guiDir = join(root, "linux", "internal", "gui");
  mkdirSync(guiDir, { recursive: true });
  writeFileSync(join(guiDir, "theme_gen.go"), go);
}

// --- Windows (WinUI 3 / Fluent) ---------------------------------------
// A XAML ResourceDictionary with ThemeDictionaries so WinUI swaps dark/light
// with the system automatically (ActualTheme). Each token becomes a Color and a
// matching SolidColorBrush ("<Name>Brush"); metrics become x:Double / CornerRadius.
// This folds Windows — previously the one hand-maintained surface — into the
// pipeline. The WinUI window keeps Fluent chrome (Mica); these supply the shared
// semantics on top.
{
  const winHex = (hex) => {
    const { r, g, b, a } = parse(hex);
    const h2 = (v) => v.toString(16).toUpperCase().padStart(2, "0");
    return `#${h2(a)}${h2(r)}${h2(g)}${h2(b)}`; // WinUI wants #AARRGGBB
  };
  const winKey = (k) => cap(camel(k));
  const themeDict = (scheme, key) => {
    let x = `    <ResourceDictionary x:Key="${key}">\n`;
    for (const [k, hex] of Object.entries(scheme)) {
      x += `      <Color x:Key="${winKey(k)}Color">${winHex(hex)}</Color>\n`;
      x += `      <SolidColorBrush x:Key="${winKey(k)}Brush" Color="{StaticResource ${winKey(k)}Color}" />\n`;
    }
    x += `    </ResourceDictionary>\n`;
    return x;
  };

  let xaml = `<!-- ${HEADER} -->\n`;
  xaml += `<ResourceDictionary\n    xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"\n    xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml">\n\n`;
  xaml += `  <ResourceDictionary.ThemeDictionaries>\n`;
  xaml += themeDict(dark, "Dark") + themeDict(light, "Default");
  xaml += `  </ResourceDictionary.ThemeDictionaries>\n\n`;
  xaml += `  <!-- Metrics (device-independent px) shared across both themes. -->\n`;
  for (const [n, px] of Object.entries(tokens.space)) xaml += `  <x:Double x:Key="Space${cap(n)}">${px}</x:Double>\n`;
  for (const [n, px] of Object.entries(tokens.radius)) xaml += `  <CornerRadius x:Key="Radius${cap(n)}">${px}</CornerRadius>\n`;
  for (const [n, px] of Object.entries(tokens.font.size)) xaml += `  <x:Double x:Key="Text${cap(n)}">${px}</x:Double>\n`;
  xaml += `</ResourceDictionary>\n`;
  writeFileSync(join(root, "windows", "Theme.xaml"), xaml);
}

console.log("tokens: wrote tokens.css, Theme.kt, Theme.swift, theme_gen.go (linux), Theme.xaml (windows)");
