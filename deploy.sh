#!/usr/bin/env bash
# Deploy abacad to production: `make deploy` (or ./deploy.sh).
#
# Ships two artifacts to $DEPLOY_HOST (default xyz-sg-1, an ~/.ssh/config host):
#
#   1. the server — Docker image built from the local tree (linux/amd64),
#      side-loaded over SSH and restarted via the compose project in
#      /root/abacad.ai. No registry round-trip.
#   2. the macOS client — a Developer ID-signed, notarized + stapled .dmg built
#      from macos/, uploaded into the served downloads dir:
#      https://abacad.ai/downloads/abacad-macos-latest.dmg
#
# It also ships the local .env as the server's production config: edit .env,
# run this, and the new values are live after the restart. See the "shipping
# .env" step for the precedence rules and what .env cannot override.
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
# macOS client signing: the team's Developer ID Application cert + a notary
# keychain profile created once with `xcrun notarytool store-credentials
# abacad-notary --key AuthKey_*.p8 --key-id … --issuer …`. Override via env.
MAC_SIGN_IDENTITY="${MAC_SIGN_IDENTITY:-Developer ID Application: Beijing Xiaoyuanzhu Technology Co., Ltd. (R3845XW5FZ)}"
MAC_NOTARY_PROFILE="${MAC_NOTARY_PROFILE:-abacad-notary}"
# The compose data volume (./data:/data), bind-mounted at /data in-container.
DATA_DIR="$COMPOSE_DIR/data"
# Must match the compose data volume + the ABACAD_DOWNLOADS path in the image.
DOWNLOADS_DIR="$DATA_DIR/abacad-downloads"
DMG="$here/macos/build/abacad.dmg"
DMG_NAME="abacad-macos-latest.dmg"
# Local config shipped to the server. Override to deploy a different file.
ENV_FILE="${DEPLOY_ENV_FILE:-$here/.env}"
# The container's uid (see the Dockerfile's `adduser -u 10001`); the shipped
# .env is chowned to it so the non-root server can read a 0600 file.
APP_UID=10001

# Build everything first — fail before touching the host.
echo "== building server image ($IMAGE, linux/amd64) =="
docker build --platform linux/amd64 -t "$IMAGE" -f "$here/server/Dockerfile" "$here/server"

echo "== building + notarizing macOS client dmg =="
make -C "$here/macos" release \
  SIGN_IDENTITY="$MAC_SIGN_IDENTITY" \
  NOTARY_PROFILE="$MAC_NOTARY_PROFILE"

echo "== shipping image to $HOST =="
docker save "$IMAGE" | gzip | ssh "$HOST" 'gunzip | docker load'

# Ship the config before the restart, so the new process starts with it. The
# server reads the nearest .env walking up from its working directory, which is
# /data (see backend/internal/config/dotenv.go + the Dockerfile's WORKDIR), so
# landing it in the data volume needs no compose change.
#
# Precedence: the real environment wins over the file, so anything set in the
# Dockerfile (ABACAD_ADDR, ABACAD_DB, ABACAD_BLOBS, ABACAD_DOWNLOADS,
# ABACAD_SSH_HOST_KEY) or in docker-compose.yml (ABACAD_SSH_ADDR,
# ABACAD_BASE_DOMAIN) is NOT overridable from .env — those belong to the
# deployment. .env carries the secrets and everything else.
env_changed=0
if [ -f "$ENV_FILE" ]; then
  echo "== shipping $(basename "$ENV_FILE") to $HOST:$DATA_DIR/.env =="
  before="$(ssh "$HOST" "md5sum $DATA_DIR/.env 2>/dev/null | cut -d' ' -f1" || true)"
  # Upload to a temp name, then rename — the server never reads a half-written file.
  scp -q "$ENV_FILE" "$HOST:$DATA_DIR/.env.tmp"
  ssh "$HOST" "chmod 600 $DATA_DIR/.env.tmp && chown $APP_UID:$APP_UID $DATA_DIR/.env.tmp && mv $DATA_DIR/.env.tmp $DATA_DIR/.env"
  after="$(ssh "$HOST" "md5sum $DATA_DIR/.env | cut -d' ' -f1")"
  [ "$before" = "$after" ] || env_changed=1
else
  echo "== no $ENV_FILE — leaving the server's existing config in place =="
fi

# `up -d` is a no-op when neither the image nor the compose file changed, and it
# can't see a changed .env (the server reads that itself, from the volume). So a
# config-only deploy has to force the recreate, or the new values never load.
echo "== restarting server on $HOST =="
if [ "$env_changed" = 1 ]; then
  ssh "$HOST" "cd $COMPOSE_DIR && docker compose up -d --force-recreate"
else
  ssh "$HOST" "cd $COMPOSE_DIR && docker compose up -d"
fi

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
