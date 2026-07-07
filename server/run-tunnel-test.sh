#!/usr/bin/env bash
# End-to-end tunnel test: boots the server (seeded, temp DB), attaches a
# mock-desktop device, and runs test-tunnel.mjs through the /connect relay.
# Everything is torn down on exit.
set -euo pipefail

cd "$(dirname "$0")"
export PATH=/tmp/goroot/bin:$PATH
PORT="${PORT:-8848}"
DB="$(mktemp -u /tmp/abacad-tunnel-XXXX.db)"
LOG="$(mktemp /tmp/abacad-tunnel-XXXX.log)"
pids=()

cleanup() {
  for p in "${pids[@]:-}"; do kill "$p" 2>/dev/null || true; done
  rm -f "$DB" "$LOG"
}
trap cleanup EXIT

# Free the port if a stale run is holding it (known gotcha on :8848).
fuser -k "${PORT}/tcp" 2>/dev/null || true
sleep 0.3

echo "== starting server (:$PORT, seeded) =="
( cd backend && go run ./cmd/abacad -seed -addr ":$PORT" -db "$DB" ) >"$LOG" 2>&1 &
pids+=($!)

# Wait for the seed tokens to appear in the log.
for _ in $(seq 1 100); do
  grep -q "mcp_token=" "$LOG" && break
  sleep 0.2
done
DEV_TOKEN="$(grep -o 'device_token=[^ ]*' "$LOG" | head -1 | cut -d= -f2)"
MCP_TOKEN="$(grep -o 'mcp_token=[^ ]*' "$LOG" | head -1 | cut -d= -f2)"
if [ -z "$DEV_TOKEN" ] || [ -z "$MCP_TOKEN" ]; then
  echo "FAIL: could not read seed tokens"; cat "$LOG"; exit 1
fi
echo "   device_token=${DEV_TOKEN:0:8}…  mcp_token=${MCP_TOKEN:0:8}…"

echo "== attaching mock-desktop =="
SERVER_URL="ws://localhost:$PORT/device?token=$DEV_TOKEN" node mock-desktop.mjs &
pids+=($!)
sleep 1  # let it connect + register

echo "== running tunnel tests =="
ABACAD_WS="ws://localhost:$PORT" ABACAD_TOKEN="$MCP_TOKEN" node test-tunnel.mjs
