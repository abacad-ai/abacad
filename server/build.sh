#!/usr/bin/env bash
# Build the abacad server: compile the dashboard SPA, embed it into the Go
# backend, and produce the single server binary.
#
#   ./build.sh            # full build -> backend/abacad
#
# Requires: node/npm and go on PATH. On the dev Linux box, see the memory note
# "abacad-go-server-build" for the Go toolchain path and module proxy.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

# Monorepo version (repo-root VERSION), stamped into the binary via ldflags so
# the MCP serverInfo / GET /api/version report the real build. "dev" if absent.
version="$(cat "$here/../VERSION" 2>/dev/null || echo dev)"

echo "== building frontend =="
cd "$here/frontend"
npm install
npm run build

echo "== embedding dist into backend =="
rm -rf "$here/backend/internal/web/dist"
cp -r "$here/frontend/dist" "$here/backend/internal/web/dist"
# Restore the tracked stub the rm -rf above just removed. It is the only tracked
# file in dist/ (everything else is gitignored) and it exists so //go:embed has a
# directory to match on a fresh clone — losing it here would show up as a
# spurious deletion in every post-build `git status`. See internal/web/.gitignore.
touch "$here/backend/internal/web/dist/.gitkeep"

echo "== building docs site =="
cd "$here/docs-site"
npm install
npm run build

echo "== embedding docs into backend =="
rm -rf "$here/backend/internal/web/docs-dist"
cp -r "$here/docs-site/dist" "$here/backend/internal/web/docs-dist"
touch "$here/backend/internal/web/docs-dist/.gitkeep"

echo "== building backend (v$version) =="
cd "$here/backend"
go build -ldflags "-X abacad/internal/version.Version=$version" -o abacad ./cmd/abacad

echo "built: $here/backend/abacad (v$version)"
