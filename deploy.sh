#!/usr/bin/env bash
# Deploy Abacad to production: `make deploy` (or ./deploy.sh).
#
# Ships two artifacts to $DEPLOY_HOST (default xyz-sg-1, an ~/.ssh/config host):
#
#   1. the server — Docker image built from the local tree (linux/amd64),
#      side-loaded over SSH and restarted via the compose project in
#      /root/abacad.ai. No registry round-trip.
#   2. the macOS client — .dmg built from macos/, uploaded into the served
#      downloads dir: https://abacad.ai/downloads/abacad-macos-latest.dmg
#
# The image keeps the tag CI pushes (ghcr.io/abacad-ai/abacad:latest), so a
# later `docker compose pull` on the host converges back to CI's build of main.
#
# Requires: docker (with buildx), the macOS toolchain (this runs the macos/
# Makefile), and ssh access to $DEPLOY_HOST.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

HOST="${DEPLOY_HOST:-xyz-sg-1}"
IMAGE="ghcr.io/abacad-ai/abacad:latest"
COMPOSE_DIR="/root/abacad.ai"
# Must match the compose data volume + the ABACAD_DOWNLOADS path in the image.
DOWNLOADS_DIR="$COMPOSE_DIR/data/abacad-downloads"
DMG="$here/macos/build/AbacadAgent.dmg"
DMG_NAME="abacad-macos-latest.dmg"

# Build everything first — fail before touching the host.
echo "== building server image ($IMAGE, linux/amd64) =="
docker build --platform linux/amd64 -t "$IMAGE" -f "$here/server/Dockerfile" "$here/server"

echo "== building macOS client dmg =="
make -C "$here/macos" dmg

echo "== shipping image to $HOST =="
docker save "$IMAGE" | gzip | ssh "$HOST" 'gunzip | docker load'

echo "== restarting server on $HOST =="
ssh "$HOST" "cd $COMPOSE_DIR && docker compose up -d"

echo "== waiting for the server to report healthy =="
status=unknown
for _ in $(seq 1 30); do
  status="$(ssh "$HOST" 'docker inspect --format {{.State.Health.Status}} abacad' 2>/dev/null || echo unknown)"
  [ "$status" = healthy ] && break
  sleep 2
done
if [ "$status" != healthy ]; then
  echo "server is not healthy after restart (status: $status); recent logs:" >&2
  ssh "$HOST" 'docker logs --tail 50 abacad' >&2 || true
  exit 1
fi

echo "== uploading $DMG_NAME =="
# Upload to a temp name, then rename — the served file is never half-written.
ssh "$HOST" "mkdir -p $DOWNLOADS_DIR"
scp -q "$DMG" "$HOST:$DOWNLOADS_DIR/.$DMG_NAME.tmp"
ssh "$HOST" "mv $DOWNLOADS_DIR/.$DMG_NAME.tmp $DOWNLOADS_DIR/$DMG_NAME"

echo "== verifying through https://abacad.ai =="
curl -sSf https://abacad.ai/health && echo
curl -sSfI "https://abacad.ai/downloads/$DMG_NAME" | grep -iE '^(HTTP|content-length|content-type)'
echo "deploy OK"
