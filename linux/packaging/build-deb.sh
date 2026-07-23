#!/usr/bin/env bash
# Assemble a Debian/Ubuntu .deb from an already-built abacad GUI binary.
#
#   packaging/build-deb.sh [path-to-binary]   (default: build/abacad-gui)
#   ARCH=arm64 packaging/build-deb.sh …        (override the package arch)
#
# Ships: /usr/bin/abacad, a .desktop launcher, a systemd *user* service, and the
# app icon. Runtime deps declare libgtk-4-1 + libadwaita-1-0. Needs dpkg-deb.
set -euo pipefail
cd "$(dirname "$0")/.."   # linux/

BIN="${1:-build/abacad-gui}"
ARCH="${ARCH:-amd64}"
VERSION="$(cat ../VERSION 2>/dev/null | tr -d '[:space:]' || echo 0.0.0)"

if [ ! -x "$BIN" ]; then
  echo "no binary at $BIN — run 'make gui' first" >&2
  exit 1
fi

ROOT="build/deb"
rm -rf "$ROOT"
install -Dm0755 "$BIN"                       "$ROOT/usr/bin/abacad"
install -Dm0644 packaging/abacad.desktop     "$ROOT/usr/share/applications/abacad.desktop"
install -Dm0644 packaging/abacad.service     "$ROOT/usr/lib/systemd/user/abacad.service"
install -Dm0644 ../assets/icon.svg           "$ROOT/usr/share/icons/hicolor/scalable/apps/abacad.svg"

mkdir -p "$ROOT/DEBIAN"
cat > "$ROOT/DEBIAN/control" <<EOF
Package: abacad
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Depends: libgtk-4-1, libadwaita-1-0
Maintainer: abacad <noreply@abacad.ai>
Homepage: https://abacad.ai
Description: abacad device agent (desktop)
 Let an AI agent see and control this machine over the abacad relay, with a
 local pause and disconnect always one click away. Ships the GTK4/libadwaita
 desktop app (abacad --gui) and a systemd user service that keeps the relay
 connection alive in the background.
EOF

OUT="build/abacad_${VERSION}_${ARCH}.deb"
dpkg-deb --build --root-owner-group "$ROOT" "$OUT"
echo "built $OUT"
