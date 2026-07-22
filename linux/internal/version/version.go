// Package version carries the single monorepo version string, shared with the
// server and every other client. The whole repo moves as one number, so there is
// no per-component semver — just this value, reported to the relay on connect.
//
// "dev" is the default for a bare `go build`; real builds stamp the repo-root
// VERSION file in via the linker:
//
//	go build -ldflags "-X abacad-linux/internal/version.Version=$(cat ../VERSION)" ./cmd/abacad
package version

// Version is the running build's version. Overridden at link time (see the
// package doc); "dev" means an unstamped local build.
var Version = "dev"
