#!/usr/bin/env bash
# Local development: a throwaway hub plus the Vite dev server with HMR.
#
#   make dev            → http://127.0.0.1:8899  (admin token: $DEV_TOKEN)
#
# The hub keeps its state in .dev/ inside the checkout, never in
# ~/.config/abacad, so a real hub on the same machine is left alone. Its raw
# VNC ports use 6901-6999 for the same reason. Ctrl-C stops both processes.
set -euo pipefail

cd "$(dirname "$0")/.."

DEV_WEB_PORT="${DEV_WEB_PORT:-8899}"
DEV_HUB_ADDR="${DEV_HUB_ADDR:-127.0.0.1:8788}"
DEV_TOKEN="${DEV_TOKEN:-abacad-dev-admin-token}"
DEV_DIR="${DEV_DIR:-.dev}"
DEV_CONFIG="$DEV_DIR/config.yaml"

for port in "$DEV_WEB_PORT" "${DEV_HUB_ADDR##*:}"; do
	if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
		echo "dev: port $port is already in use:" >&2
		lsof -nP -iTCP:"$port" -sTCP:LISTEN >&2 || true
		exit 1
	fi
done

mkdir -p "$DEV_DIR"
if [ ! -f "$DEV_CONFIG" ]; then
	cat >"$DEV_CONFIG" <<EOF
listen: $DEV_HUB_ADDR
vnc_port_start: 6901
vnc_port_end: 6999
devices: []
EOF
fi

if [ ! -d frontend/node_modules ]; then
	(cd frontend && npm ci)
fi

pids=()
cleanup() {
	trap - EXIT INT TERM
	for pid in ${pids[@]+"${pids[@]}"}; do
		kill "$pid" 2>/dev/null || true
	done
	wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Build first and run the binary itself: `go run` would leave the hub orphaned
# when the script is stopped.
go build -o "$DEV_DIR/abacad" ./cmd/abacad
ABACAD_ADMIN_TOKEN="$DEV_TOKEN" \
	"$DEV_DIR/abacad" serve --config "$DEV_CONFIG" --listen "$DEV_HUB_ADDR" -v &
pids+=($!)

(cd frontend && ABACAD_DEV_API="http://$DEV_HUB_ADDR" \
	exec ./node_modules/.bin/vite --host 127.0.0.1 --port "$DEV_WEB_PORT" --strictPort) &
pids+=($!)

cat <<EOF

  abacad dev
  → web  http://127.0.0.1:$DEV_WEB_PORT   (Vite, HMR)
  → hub  http://$DEV_HUB_ADDR    (API/MCP/WS, state in $DEV_DIR/)
  admin token: $DEV_TOKEN

EOF

# Either process exiting takes the other one down with it. (macOS ships
# bash 3.2, which has no `wait -n`.)
while kill -0 "${pids[0]}" 2>/dev/null && kill -0 "${pids[1]}" 2>/dev/null; do
	sleep 1
done
