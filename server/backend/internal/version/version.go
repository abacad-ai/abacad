// Package version carries the single monorepo version string, shared by the
// server and every client. The whole repo moves as one number (a release bumps
// every component together, even ones with no code change), so there is no
// per-component semver to reconcile — just this one value.
//
// The default below is a placeholder for `go run`/`go test` and bare `go build`.
// Real builds stamp the repo-root VERSION file in via the linker:
//
//	go build -ldflags "-X abacad/internal/version.Version=$(cat VERSION)" ./cmd/abacad
package version

// Version is the running build's version. Overridden at link time (see the
// package doc); "dev" means an unstamped local build.
var Version = "dev"
