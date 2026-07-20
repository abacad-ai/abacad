#!/usr/bin/env bash
# End-to-end check for the browser device surface on this Linux box:
# build -> seed a server -> connect the browser mock -> drive it as an agent.
# No ps/pgrep on the box, so we track PIDs we spawn and kill those.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
export PATH=/tmp/goroot/bin:$PATH GOFLAGS=-mod=mod
PORT=8853
BASE="http://localhost:${PORT}"
TMP="$(mktemp -d)"
LOG="$TMP/server.log"
BIN="$TMP/abacad"

cleanup() {
  [ -n "${MOCK_PID:-}" ] && kill "$MOCK_PID" 2>/dev/null
  [ -n "${SRV_PID:-}" ] && kill "$SRV_PID" 2>/dev/null
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "== build =="
( cd "$HERE/backend" && go build -o "$BIN" ./cmd/abacad ) || { echo "build failed"; exit 1; }

echo "== boot server (:$PORT, seeded) =="
"$BIN" -addr ":$PORT" -db "$TMP/x.db" -blobs "$TMP/blobs" -screenshots "$TMP/shots" -seed >"$LOG" 2>&1 &
SRV_PID=$!

for i in $(seq 1 50); do
  curl -sf "$BASE/health" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -sf "$BASE/health" >/dev/null 2>&1 || { echo "server never came up"; cat "$LOG"; exit 1; }

DEV_TOKEN=$(grep -oE 'SEED device_token=[^ ]+' "$LOG" | head -1 | cut -d= -f2)
MCP_TOKEN=$(grep -oE 'SEED mcp_token=[^ ]+' "$LOG" | head -1 | cut -d= -f2)
[ -n "$DEV_TOKEN" ] && [ -n "$MCP_TOKEN" ] || { echo "could not read seed tokens"; cat "$LOG"; exit 1; }
echo "   device_token=${DEV_TOKEN:0:12}...  mcp_token=${MCP_TOKEN:0:12}..."

echo "== connect browser mock =="
SERVER_URL="ws://localhost:${PORT}/device?token=${DEV_TOKEN}" node "$HERE/mock-browser.mjs" 2>"$TMP/mock.log" &
MOCK_PID=$!

echo "== run agent smoke =="
MCP_URL="$BASE/mcp" MCP_TOKEN="$MCP_TOKEN" node "$HERE/smoke-browser.mjs"
RC=$?

exit $RC
