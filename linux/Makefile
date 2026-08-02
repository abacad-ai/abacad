# abacad Linux client. Pure-Go X11 (no cgo, no libX11 dev headers, no xdotool)
# for the CLI; the `gui` target below adds a GTK4 front-end behind a build tag.

BIN := build/abacad

# Version stamp, passed down from the repo-root Makefile (which owns VERSION).
# Empty on a bare `make -C linux build`, which reports the built-in default.
GO_LDFLAGS ?=

.PHONY: build run test vet clean xvfb-test deps-gui gui deb

# Build the daemon. Output: build/abacad
build:
	go build -ldflags "$(GO_LDFLAGS)" -o $(BIN) ./cmd/abacad

# Run against a relay. Pass the URL: make run URL=wss://host/device?token=…
run: build
	./$(BIN) --server-url "$(URL)"

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf build

# Headless end-to-end check under a virtual X server (Xvfb): starts a mock
# /device relay, drives every verb, asserts the replies. Needs Xvfb on PATH.
xvfb-test:
	go test -tags e2e -run TestXvfbE2E -v ./internal/e2e

# --- GUI (GTK4 / libadwaita, cgo) -------------------------------------------
# The desktop app (abacad --gui). Needs libgtk-4-dev + libadwaita-1-dev (>= 1.4,
# for AdwToolbarView) on the build host. Built with the `gui` tag; the default
# `build` target above stays cgo-free for servers/CI (uses internal/gui/stub.go).
#
# The gotk4 bindings are ordinary pinned requirements in go.mod/go.sum, like every
# other dependency — `gui` does NOT fetch them. (It used to depend on deps-gui,
# which ran `go get` at build time; that made the build unreproducible and meant a
# breaking upstream release would land silently in the next build.) Use deps-gui
# by hand to bump them, then commit the go.mod/go.sum it writes.
GUI_BIN := build/abacad-gui

deps-gui:
	go get github.com/diamondburned/gotk4/pkg/gtk/v4 \
	       github.com/diamondburned/gotk4/pkg/gio/v2 \
	       github.com/diamondburned/gotk4/pkg/glib/v2 \
	       github.com/diamondburned/gotk4-adwaita/pkg/adw
	go mod tidy

gui:
	CGO_ENABLED=1 go build -tags gui -ldflags "$(GO_LDFLAGS)" -o $(GUI_BIN) ./cmd/abacad

# Build a Debian/Ubuntu .deb: the GUI binary + a systemd user service + a desktop
# launcher + icon. Needs dpkg-deb. Output: build/abacad_<version>_amd64.deb
deb: gui
	packaging/build-deb.sh $(GUI_BIN)
