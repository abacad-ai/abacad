# abacad Linux client. Pure-Go X11 (no cgo, no libX11 dev headers, no xdotool).

BIN := build/abacad

.PHONY: build run test vet clean xvfb-test deps-gui gui deb

# Build the daemon. Output: build/abacad
build:
	go build -o $(BIN) ./cmd/abacad

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
# The desktop app (abacad --gui). Needs libgtk-4-dev + libadwaita-1-dev and the
# gotk4 bindings (fetched on first build). Built with the `gui` tag; the default
# `build` target above stays cgo-free for servers/CI (uses internal/gui/stub.go).
GUI_BIN := build/abacad-gui

# Fetch the gotk4 + libadwaita Go bindings and pin them into go.mod/go.sum.
deps-gui:
	go get github.com/diamondburned/gotk4/pkg/gtk/v4 \
	       github.com/diamondburned/gotk4/pkg/gio/v2 \
	       github.com/diamondburned/gotk4/pkg/glib/v2 \
	       github.com/diamondburned/gotk4-adwaita/pkg/adw
	go mod tidy

gui: deps-gui
	CGO_ENABLED=1 go build -tags gui -o $(GUI_BIN) ./cmd/abacad

# Build a Debian/Ubuntu .deb: the GUI binary + a systemd user service + a desktop
# launcher + icon. Needs dpkg-deb. Output: build/abacad_<version>_amd64.deb
deb: gui
	packaging/build-deb.sh $(GUI_BIN)
