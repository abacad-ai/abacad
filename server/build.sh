#!/usr/bin/env bash
# Build the Abacad server: compile the dashboard SPA, embed it into the Go
# backend, and produce the single server binary.
#
#   ./build.sh            # full build -> backend/abacad
#
# Requires: node/npm and go on PATH. On the dev Linux box, see the memory note
# "abacad-go-server-build" for the Go toolchain path and module proxy.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

echo "== building frontend =="
cd "$here/frontend"
npm install
npm run build

echo "== embedding dist into backend =="
rm -rf "$here/backend/internal/web/dist"
cp -r "$here/frontend/dist" "$here/backend/internal/web/dist"

echo "== building backend =="
cd "$here/backend"
go build -o abacad ./cmd/abacad

echo "built: $here/backend/abacad"
