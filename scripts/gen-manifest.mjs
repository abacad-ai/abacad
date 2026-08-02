#!/usr/bin/env node
// Generate the client downloads manifest from a directory of release artifacts.
//
//   node scripts/gen-manifest.mjs <dir>
//
// The <dir> holds artifacts named by the repo-wide convention
//
//   abacad-<kind>-<version>-<platform>-<arch>.<suffix>
//     e.g. abacad-app-0.5.0-macos-apple-silicon.dmg
//          abacad-cli-0.5.0-macos-apple-silicon.tar.gz
//          abacad-app-0.5.0-linux-amd64.deb
//          abacad-cli-0.5.0-linux-amd64.tar.gz
//          abacad-app-0.5.0-android-universal.apk
//          abacad-app-0.5.0-windows-x64.exe
//          abacad-cli-0.5.0-windows-x64.zip
//
// and this writes <dir>/manifest.json describing them. The manifest is the one
// thing every consumer reads — the downloads page renders from it, install.sh
// greps the Linux CLI tarball URL out of it, and a future in-app auto-updater can
// diff its own version against it (that's why each build carries a sha256).
//
// It is a static file that travels *with* the artifacts: `make build release`
// stages both into the server's downloads dir, and CI regenerates it over the
// gathered release assets. The server never scans — it just serves this file.
//
// Only the newest build per <kind>-<platform>-<arch> is listed, so dropping a
// newer version in and regenerating supersedes the old one; older versioned files
// stay downloadable by direct URL but drop off the manifest. `kind` is part of the
// key, not decoration: most platforms publish an app *and* a CLI for the same
// platform+arch, and keying on platform+arch alone would silently drop one of
// them (a linux-amd64 .deb and .tar.gz would evict each other).
//
// Artifacts that predate the <kind> segment no longer match and are ignored —
// they stay reachable by direct URL, the same as any superseded build.

import { createHash } from "node:crypto";
import { readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const dir = process.argv[2];
if (!dir) {
  console.error("usage: node scripts/gen-manifest.mjs <dir>");
  process.exit(1);
}

// Versions are strictly x.y.z (enforced by `make bump-version`), so the version
// field never contains a dash and this left-to-right parse is unambiguous even
// with the leading kind. The suffix swallows the rest (handles ".tar.gz").
//
// Arch admits dashes, because the arch token is whatever that platform's own
// users call it — `macos-apple-silicon`, not `macos-arm64` — and Apple's word for
// it has a dash in it. Platform stays dash-free, which is what keeps the parse
// unambiguous: arch is everything between the platform and the first dot.
const NAME = /^abacad-(app|cli)-(\d+\.\d+\.\d+)-([a-z]+)-([a-z0-9][a-z0-9-]*)\.([a-z0-9.]+)$/;

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

// Keep only the newest version per kind-platform-arch.
const latest = new Map(); // "kind-platform-arch" -> build
for (const name of entries) {
  const m = NAME.exec(name);
  if (!m) continue; // manifest.json, checksums, older stray files — ignored
  const [, kind, version, platform, arch] = m;
  const path = join(dir, name);
  const st = statSync(path);
  if (!st.isFile()) continue;
  const key = `${kind}-${platform}-${arch}`;
  const prev = latest.get(key);
  if (prev && cmpVersion(version, prev.version) <= 0) continue;
  latest.set(key, {
    kind,
    platform,
    arch,
    version,
    file: name,
    url: `/downloads/${name}`,
    size: st.size,
    sha256: createHash("sha256").update(readFileSync(path)).digest("hex"),
  });
}

// Grouped the way the downloads page reads them: one platform at a time, app
// before cli ("app" sorts first), then by arch.
const builds = [...latest.values()].sort(
  (a, b) =>
    a.platform.localeCompare(b.platform) || a.kind.localeCompare(b.kind) || a.arch.localeCompare(b.arch),
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
for (const b of builds) console.log(`  ${b.kind}  ${b.platform}-${b.arch}  ${b.file}`);
