#!/usr/bin/env node
/**
 * The vendored noVNC tree (public/novnc) is the upstream release, with exactly
 * one change of ours: `app/ui.js` never registers the `beforeunload` handler, so
 * closing the hub tab does not ask "Leave site?" for a screen that has nothing
 * to save. A re-vendor copies the tree over that line and the prompt comes back
 * — this check makes that fail the build instead of surfacing as a bug report.
 *
 * See docs/vendored-novnc.md ("Patches").
 */
import { readdirSync, readFileSync, statSync } from "node:fs"
import { join, relative } from "node:path"
import { fileURLToPath } from "node:url"

const tree = fileURLToPath(new URL("../public/novnc/", import.meta.url))
const root = fileURLToPath(new URL("../", import.meta.url))

// Anything that would bring the prompt back: addEventListener/attachEvent on
// "beforeunload" (either quoting) or an onbeforeunload assignment.
const arms = [
  /addEventListener\(\s*["']beforeunload["']/,
  /attachEvent\(\s*["']onbeforeunload["']/,
  /\bonbeforeunload\s*=(?!=)/,
]

function walk(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return walk(path)
    return /\.(js|mjs|html)$/.test(name) ? [path] : []
  })
}

const findings = []
for (const file of walk(tree)) {
  readFileSync(file, "utf8")
    .split("\n")
    .forEach((line, index) => {
      if (/^\s*(\/\/|\*|\/\*)/.test(line)) return
      if (arms.some((arm) => arm.test(line))) {
        findings.push(`${relative(root, file)}:${index + 1}  ${line.trim()}`)
      }
    })
}

if (findings.length > 0) {
  console.error(
    [
      "The vendored noVNC tree registers a beforeunload handler again:",
      "",
      ...findings.map((finding) => `  ${finding}`),
      "",
      "Closing the hub tab must not ask \"Leave site?\". Drop the registration",
      "in app/ui.js (docs/vendored-novnc.md, \"Patches\") and re-run the build.",
    ].join("\n"),
  )
  process.exit(1)
}

console.log("novnc: no beforeunload handler in the vendored tree")
