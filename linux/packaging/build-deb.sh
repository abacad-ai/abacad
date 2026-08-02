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
# Default to the build host's own architecture, not a hardcoded amd64: the GUI
# links cgo against the host's GTK, so the binary is always native, and a fixed
# default would label an arm64 build "amd64" — a package that installs happily
# and then refuses to exec.
ARCH="${ARCH:-$(dpkg --print-architecture)}"
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
Depends: libgtk-4-1, libadwaita-1-0 (>= 1.4)
Maintainer: abacad <noreply@abacad.ai>
Homepage: https://abacad.ai
Description: abacad device agent (desktop)
 Let an AI agent see and control this machine over the abacad relay, with a
 local pause and disconnect always one click away. Ships the GTK4/libadwaita
 desktop app (abacad --gui) and a systemd user service that keeps the relay
 connection alive in the background.
EOF

# The window uses AdwToolbarView, which is libadwaita 1.4+. Without the floor
# above, apt would happily install against an older libadwaita and the binary
# would fail to resolve adw_toolbar_view_new at load time — a dependency error is
# a much better failure than that.

# Refresh the caches the freshly-installed .desktop file and icon land in;
# without this the launcher can take a re-login to appear.
cat > "$ROOT/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = configure ]; then
	command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications || true
	command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -qtf /usr/share/icons/hicolor || true
fi
EOF
chmod 0755 "$ROOT/DEBIAN/postinst"

OUT="build/abacad_${VERSION}_${ARCH}.deb"
dpkg-deb --build --root-owner-group "$ROOT" "$OUT"
echo "built $OUT"
