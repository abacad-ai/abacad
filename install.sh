#!/bin/sh
# abacad Linux client installer.
#
#   curl -fsSL https://abacad.ai/install.sh | sh
#
# Downloads the right prebuilt `abacad` binary for this machine, drops it on your
# PATH, and points you at `abacad connect`. Override the server (self-hosted) with
#   ABACAD_SERVER=https://my.host  sh install.sh
set -eu

SERVER="${ABACAD_SERVER:-https://abacad.ai}"
SERVER="${SERVER%/}"

os="$(uname -s)"
arch="$(uname -m)"

if [ "$os" != "Linux" ]; then
	echo "This installer is for Linux (detected: $os)." >&2
	echo "For macOS or Windows, see $SERVER/downloads." >&2
	exit 1
fi

case "$arch" in
	x86_64 | amd64) A=amd64 ;;
	aarch64 | arm64) A=arm64 ;;
	*)
		echo "Unsupported architecture: $arch (need x86_64 or arm64)." >&2
		exit 1
		;;
esac

asset="abacad-linux-$A"
url="$SERVER/downloads/$asset"

# Prefer a system bin dir; fall back to a user-local one that needs no root.
SUDO=""
if [ -w /usr/local/bin ]; then
	bindir=/usr/local/bin
elif command -v sudo >/dev/null 2>&1; then
	bindir=/usr/local/bin
	SUDO=sudo
else
	bindir="$HOME/.local/bin"
	mkdir -p "$bindir"
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

echo "Downloading $url …"
if ! curl -fsSL "$url" -o "$tmp"; then
	echo "Download failed. Is $SERVER reachable, and is the $A build published?" >&2
	exit 1
fi
chmod +x "$tmp"
$SUDO mv "$tmp" "$bindir/abacad"
trap - EXIT

echo "Installed abacad to $bindir/abacad"
case ":$PATH:" in
	*":$bindir:"*) ;;
	*) echo "Note: $bindir is not on your PATH — add it, or run $bindir/abacad directly." ;;
esac

echo
if [ "$SERVER" = "https://abacad.ai" ]; then
	echo "Next:  abacad connect"
else
	echo "Next:  abacad connect --server $SERVER"
fi
