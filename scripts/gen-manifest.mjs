#!/usr/bin/env node
// Generate the client downloads manifest from a directory of release artifacts.
//
//   node scripts/gen-manifest.mjs <dir>
//
// The <dir> holds artifacts named by the repo-wide convention
//
//   abacad-<version>-<platform>-<arch>.<suffix>
//     e.g. abacad-0.4.0-macos-arm64.dmg
//          abacad-0.4.0-linux-amd64.tar.gz
//          abacad-0.4.0-android-universal.apk
//          abacad-0.4.0-windows-amd64.exe
//
// and this writes <dir>/manifest.json describing them. The manifest is the one
// thing every consumer reads — the downloads page renders from it, install.sh
// greps the Linux tarball URL out of it, and a future in-app auto-updater can
// diff its own version against it (that's why each build carries a sha256).
//
// It is a static file that travels *with* the artifacts: `make build release`
// stages both into the server's downloads dir, and CI regenerates it over the
// gathered release assets. The server never scans — it just serves this file.
//
// Only the newest build per <platform>-<arch> is listed, so dropping a newer
// version in and regenerating supersedes the old one; older versioned files stay
// downloadable by direct URL but drop off the manifest.

import { createHash } from "node:crypto";
import { readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const dir = process.argv[2];
if (!dir) {
  console.error("usage: node scripts/gen-manifest.mjs <dir>");
  process.exit(1);
}

// Versions are strictly x.y.z (enforced by `make bump-version`), so the version
// field never contains a dash and this left-to-right parse is unambiguous. The
// suffix swallows the rest (handles the two-dot ".tar.gz").
const NAME = /^abacad-(\d+\.\d+\.\d+)-([a-z]+)-([a-z0-9]+)\.([a-z0-9.]+)$/;

// Numeric semver compare for x.y.z. Returns >0 when a is newer than b.
function cmpVersion(a, b) {
  const pa = a.split(".").map(Number);
  const pb = b.split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] - pb[i];
  }
  return 0;
}

let entries;
try {
  entries = readdirSync(dir);
} catch (err) {
  console.error(`gen-manifest: cannot read ${dir}: ${err.message}`);
  process.exit(1);
}

// Keep only the newest version per platform-arch.
const latest = new Map(); // "platform-arch" -> build
for (const name of entries) {
  const m = NAME.exec(name);
  if (!m) continue; // manifest.json, checksums, older stray files — ignored
  const [, version, platform, arch] = m;
  const path = join(dir, name);
  const st = statSync(path);
  if (!st.isFile()) continue;
  const key = `${platform}-${arch}`;
  const prev = latest.get(key);
  if (prev && cmpVersion(version, prev.version) <= 0) continue;
  latest.set(key, {
    platform,
    arch,
    version,
    file: name,
    url: `/downloads/${name}`,
    size: st.size,
    sha256: createHash("sha256").update(readFileSync(path)).digest("hex"),
  });
}

const builds = [...latest.values()].sort(
  (a, b) => a.platform.localeCompare(b.platform) || a.arch.localeCompare(b.arch),
);

// Top-level version = newest across all builds. The whole monorepo ships one
// number, so in practice every build agrees; taking the max is just robust to a
// half-finished publish where one platform's file landed before another's.
const version = builds.reduce((v, b) => (v && cmpVersion(v, b.version) >= 0 ? v : b.version), "");

const manifest = {
  version,
  generated_at: Math.floor(Date.now() / 1000),
  builds,
};

writeFileSync(join(dir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
console.log(`gen-manifest: ${builds.length} build(s) → ${join(dir, "manifest.json")} (v${version || "none"})`);
for (const b of builds) console.log(`  ${b.platform}-${b.arch}  ${b.file}`);
