#!/usr/bin/env bash
# Start the iOS bridge + RFB server.
#
#   ./serve-ios.sh                          # listen on all interfaces, port 5970
#   ./serve-ios.sh 192.168.1.10:5970        # explicit bind
#   ./serve-ios.sh 0.0.0.0:5970 --password  # require a VNC password
#
# Needs the venv (.venv) with pymobiledevice3, and a paired, unlocked device on
# USB. The device is kept awake by an assertion on the device itself, so the
# screen will not lock mid-session.
#
# Any further arguments go to "iosvnc serve" (-password, -quality, -name, ...).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
addr=${1:-0.0.0.0:5970}
[ $# -gt 0 ] && shift
case "$addr" in
  127.0.0.1:*|localhost:*) ;;
  *)
    # Anyone on the LAN who can reach this port can drive the phone. Say so
    # once, loudly, instead of letting the default quietly become public.
    if [[ ! " $* " =~ [[:space:]]-?-?password[[:space:]=] ]]; then
      echo "warning: $addr is reachable from the network and no --password was given;" >&2
      echo "         pass --password <pw> or bind 127.0.0.1:5970" >&2
    fi
    ;;
esac
cd "$here"
# Always rebuild: the Go build cache makes this a no-op when nothing changed,
# and a stale binary here is indistinguishable from a bug in the bridge.
go build -o iosvnc ./cmd/iosvnc
exec ./iosvnc serve \
  --helper "$here/bridge/iosbridge.py" \
  --helper-python "$here/.venv/bin/python" \
  --listen "$addr" --scale 1.0 "$@"
