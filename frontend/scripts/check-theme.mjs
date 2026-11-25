#!/usr/bin/env node
/**
 * The theme has one source of truth: src/index.css. Components use its tokens
 * (bg-card, border-border, text-muted-foreground, bg-brand, shadow-pill, …) and
 * never carry a colour of their own — a raw hex here is a second palette nobody
 * decided on.
 *
 * The brand mark is the exception: its two fills are the mark, not the theme.
 * See brand/theme.md.
 */
import { readdirSync, readFileSync, statSync } from "node:fs"
import { join, relative } from "node:path"
import { fileURLToPath } from "node:url"

const root = fileURLToPath(new URL("../src/", import.meta.url))
const allowed = new Set(["components/brand-mark.tsx"])
const colour = /#[0-9a-fA-F]{3,8}\b|\brgba?\(|\bhsla?\(/g

function walk(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return walk(path)
    return /\.(tsx?|jsx?)$/.test(name) ? [path] : []
  })
}

const findings = []
for (const file of walk(root)) {
  const rel = relative(root, file)
  if (allowed.has(rel)) continue
  const lines = readFileSync(file, "utf8").split("\n")
  lines.forEach((line, index) => {
    for (const match of line.matchAll(colour)) {
      findings.push(`${rel}:${index + 1}  ${match[0]}`)
    }
  })
}

if (findings.length > 0) {
  console.error(
    [
      "Raw colours in components — the theme lives in frontend/src/index.css:",
      "",
      ...findings.map((finding) => `  ${finding}`),
      "",
      "Use a token (bg-card, text-muted-foreground, bg-brand, border-border,",
      "shadow-card, …) or add the value to index.css first. brand/theme.md has",
      "the palette and the rules.",
    ].join("\n"),
  )
  process.exit(1)
}

console.log("theme: no raw colours outside src/index.css")
