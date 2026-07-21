# abacad Linux client. Pure-Go X11 (no cgo, no libX11 dev headers, no xdotool).

BIN := build/abacad

.PHONY: build run test vet clean xvfb-test

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
