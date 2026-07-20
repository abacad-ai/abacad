#!/usr/bin/env bash
# Live verification: boot a seeded server, then run the REAL browser client in a
# headless Chromium (Playwright) and drive it as an agent over MCP.
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
export PATH=/tmp/goroot/bin:$PATH GOFLAGS=-mod=mod
PORT=8856
BASE="http://localhost:${PORT}"
TMP="$(mktemp -d)"; LOG="$TMP/server.log"; BIN="$TMP/abacad"

cleanup() { [ -n "${SRV_PID:-}" ] && kill "$SRV_PID" 2>/dev/null; rm -rf "$TMP"; }
trap cleanup EXIT

echo "== build =="
( cd "$HERE/backend" && go build -o "$BIN" ./cmd/abacad ) || { echo "build failed"; exit 1; }

echo "== boot server (:$PORT, seeded) =="
"$BIN" -addr ":$PORT" -db "$TMP/x.db" -blobs "$TMP/blobs" -screenshots "$TMP/shots" -seed >"$LOG" 2>&1 &
SRV_PID=$!
for i in $(seq 1 50); do curl -sf "$BASE/health" >/dev/null 2>&1 && break; sleep 0.1; done
curl -sf "$BASE/health" >/dev/null 2>&1 || { echo "server never came up"; cat "$LOG"; exit 1; }

MCP_TOKEN=$(grep -oE 'SEED api_key=[^ ]+' "$LOG" | head -1 | cut -d= -f2)
[ -n "$MCP_TOKEN" ] || { echo "no seed api key"; cat "$LOG"; exit 1; }

# The seed device is addressed by its id (its subdomain), not a token. Look it up.
JAR="$TMP/jar"
curl -s -c "$JAR" -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"dev@abacad.local","password":"devpass"}' >/dev/null
DEV_ID=$(curl -s -b "$JAR" "$BASE/api/devices" | node -e 'const a=JSON.parse(require("fs").readFileSync(0));process.stdout.write((a[0]&&a[0].id)||"")')
[ -n "$DEV_ID" ] || { echo "could not read seed device id"; exit 1; }
echo "   device id: $DEV_ID  →  http://$DEV_ID.localhost:$PORT/"

echo "== drive the real client via its subdomain in headless Chromium =="
# Chromium resolves *.localhost to 127.0.0.1; the Host (<id>.localhost:PORT) carries the id.
BASE="$BASE" DEVICE_URL="http://$DEV_ID.localhost:$PORT/" MCP_TOKEN="$MCP_TOKEN" node "$HERE/verify-browser-live.mjs"
