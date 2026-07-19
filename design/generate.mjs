#!/usr/bin/env node
// Generate per-platform design tokens from design/tokens.json — the single
// source of truth for Abacad's visual language across the web dashboard,
// the Android probe, and the macOS agent.
//
//   node design/generate.mjs
//
// Outputs (all committed, all marked GENERATED):
//   server/frontend/src/tokens.css                       CSS custom properties
//   android/app/src/main/java/dev/abacad/probe/Theme.kt  Kotlin object (ARGB ints, dp/sp)
//   macos/Sources/AbacadAgent/Theme.swift                SwiftUI Colors + CGFloats

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const tokens = JSON.parse(readFileSync(join(root, "design", "tokens.json"), "utf8"));

const HEADER = "GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs";

// "#rrggbb" or "#rrggbbaa" → { r, g, b, a } with 0-255 channels.
function parse(hex) {
  const h = hex.replace("#", "");
  return {
    r: parseInt(h.slice(0, 2), 16),
    g: parseInt(h.slice(2, 4), 16),
    b: parseInt(h.slice(4, 6), 16),
    a: h.length === 8 ? parseInt(h.slice(6, 8), 16) : 255,
  };
}

const upperSnake = (name) => name.replace(/-/g, "_").toUpperCase();
const camel = (name) => name.replace(/-(\w)/g, (_, c) => c.toUpperCase());

// --- CSS ---------------------------------------------------------------
{
  let css = `/* ${HEADER} */\n:root {\n`;
  for (const [name, hex] of Object.entries(tokens.color)) {
    const { a } = parse(hex);
    // CSS keeps the source notation; 8-digit hex is valid CSS Color 4.
    css += `  --${name}: ${hex};\n`;
    if (a !== 255 && hex.length !== 9) throw new Error(`bad hex for ${name}`);
  }
  for (const [name, px] of Object.entries(tokens.space)) css += `  --space-${name}: ${px}px;\n`;
  for (const [name, px] of Object.entries(tokens.radius)) css += `  --radius-${name}: ${px}px;\n`;
  for (const [name, px] of Object.entries(tokens.font.size)) css += `  --text-${name}: ${px}px;\n`;
  css += `}\n`;
  writeFileSync(join(root, "server", "frontend", "src", "tokens.css"), css);
}

// --- Kotlin ------------------------------------------------------------
{
  let kt = `package dev.abacad.probe\n\n// ${HEADER}\n\n`;
  kt += `/**\n * Abacad design tokens. Colors are ARGB ints (pass straight to setTextColor /\n * setBackgroundColor); SPACE_* are dp, TEXT_* are sp — multiply dp values by\n * displayMetrics.density before use.\n */\nobject Theme {\n`;
  for (const [name, hex] of Object.entries(tokens.color)) {
    const { r, g, b, a } = parse(hex);
    const argb = ((a << 24) | (r << 16) | (g << 8) | b) >>> 0;
    kt += `    const val ${upperSnake(name)} = 0x${argb.toString(16).toUpperCase().padStart(8, "0")}.toInt()\n`;
  }
  kt += `\n`;
  for (const [name, px] of Object.entries(tokens.space)) kt += `    const val SPACE_${upperSnake(name)} = ${px} // dp\n`;
  for (const [name, px] of Object.entries(tokens.radius)) kt += `    const val RADIUS_${upperSnake(name)} = ${px} // dp\n`;
  for (const [name, px] of Object.entries(tokens.font.size)) kt += `    const val TEXT_${upperSnake(name)} = ${px}f // sp\n`;
  kt += `}\n`;
  writeFileSync(join(root, "android", "app", "src", "main", "java", "dev", "abacad", "probe", "Theme.kt"), kt);
}

// --- Swift -------------------------------------------------------------
{
  let sw = `import SwiftUI\n\n// ${HEADER}\n\n`;
  sw += `/// Abacad design tokens. The menu-bar panel keeps native macOS materials for\n/// its chrome; these tokens supply the shared semantic colors (status, brand)\n/// and metrics so the panel reads as the same product as the dashboard.\nenum Theme {\n`;
  for (const [name, hex] of Object.entries(tokens.color)) {
    const { r, g, b, a } = parse(hex);
    const f = (v) => (v / 255).toFixed(4);
    sw += `    static let ${camel(name)} = Color(.sRGB, red: ${f(r)}, green: ${f(g)}, blue: ${f(b)}, opacity: ${f(a)})\n`;
  }
  sw += `\n`;
  for (const [name, px] of Object.entries(tokens.space)) sw += `    static let space${upperSnake(name).charAt(0) + camel(name).slice(1).toLowerCase()}: CGFloat = ${px}\n`;
  for (const [name, px] of Object.entries(tokens.radius)) sw += `    static let radius${upperSnake(name).charAt(0) + camel(name).slice(1).toLowerCase()}: CGFloat = ${px}\n`;
  for (const [name, px] of Object.entries(tokens.font.size)) sw += `    static let text${upperSnake(name).charAt(0) + camel(name).slice(1).toLowerCase()}: CGFloat = ${px}\n`;
  sw += `}\n`;
  writeFileSync(join(root, "macos", "Sources", "AbacadAgent", "Theme.swift"), sw);
}

console.log("tokens: wrote tokens.css, Theme.kt, Theme.swift");
