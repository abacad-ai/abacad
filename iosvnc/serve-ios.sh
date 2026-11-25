#!/usr/bin/env bash
# Start the iOS bridge + RFB server.
#
#   ./serve-ios.sh                    # listen on all interfaces, port 5970
#   ./serve-ios.sh 192.168.1.10:5970  # explicit bind
#
# Needs the venv (.venv) with pymobiledevice3, and a paired, unlocked device on
# USB. The device is kept awake by an assertion on the device itself, so the
# screen will not lock mid-session.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
addr=${1:-0.0.0.0:5970}
cd "$here"
[ -x ./iosvnc ] || go build -o iosvnc ./cmd/iosvnc
exec ./iosvnc serve \
  --helper "$here/bridge/iosbridge.py" \
  --helper-python "$here/.venv/bin/python" \
  --listen "$addr" --scale 1.0 "$@"
