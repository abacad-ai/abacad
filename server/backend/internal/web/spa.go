// Package web serves the built dashboard SPA from an embedded copy of the Vite
// dist directory. The build script copies frontend/dist here before `go build`
// (embed can't reach outside the module). Unknown paths fall back to index.html
// so client-side routes (e.g. /settings) work on hard refresh.
//
// The build output is NOT committed — see this package's .gitignore. Only a
// dist/.gitkeep stub is tracked, because //go:embed resolves at compile time and
// a missing directory is a compile error, not a runtime one. That stub keeps
// `go build`/`vet`/`test` and the IDE working on a fresh clone; New() then falls
// back to placeholderIndex below instead of refusing to boot.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// SPA is an http.Handler that serves static assets and falls back to index.html.
type SPA struct {
	fsys       fs.FS
	fileServer http.Handler
	index      []byte
	haveApp    bool // false when dist holds only the placeholder (no real build yet)
}

// placeholderIndex is served when no dashboard build is embedded. It lives in Go
// source rather than in a committed dist/index.html so that the real Vite build
// never overwrites a tracked file — dist/ stays entirely generated.
const placeholderIndex = `<!doctype html>
<meta charset="utf-8">
<title>abacad — no dashboard build</title>
<body style="font:16px/1.6 system-ui,sans-serif;max-width:34rem;margin:15vh auto;padding:0 1.5rem">
<h1 style="font-size:1.25rem">No dashboard build embedded</h1>
<p>The server is running, but <code>internal/web/dist</code> holds no build — it is
generated, never committed.</p>
<ul>
<li><code>make dev</code> — backend plus the Vite dev server (open the port it prints)</li>
<li><code>make server</code> — build the SPA, docs, and a single binary with both embedded</li>
</ul>
<p>The API, MCP endpoint, and device channels are unaffected.</p>
`

// New builds the SPA handler from the embedded dist. A dist holding only the
// .gitkeep stub is not an error: the handler serves placeholderIndex and reports
// HaveApp() == false, so a fresh clone can `go run ./cmd/abacad` without first
// building the frontend.
func New() (*SPA, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return &SPA{
			fsys:       sub,
			fileServer: http.FileServer(http.FS(sub)),
			index:      []byte(placeholderIndex),
			haveApp:    false,
		}, nil
	}
	_, statErr := fs.Stat(sub, "assets")
	return &SPA{
		fsys:       sub,
		fileServer: http.FileServer(http.FS(sub)),
		index:      index,
		haveApp:    statErr == nil,
	}, nil
}

// HaveApp reports whether a real build (with an assets/ dir) is embedded, vs the
// placeholder. Lets the server log a hint in dev (see cmd/abacad/main.go).
func (s *SPA) HaveApp() bool { return s.haveApp }

func (s *SPA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clean := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == "." || clean == "" {
		s.serveIndex(w)
		return
	}
	if f, err := s.fsys.Open(clean); err == nil {
		_ = f.Close()
		s.fileServer.ServeHTTP(w, r)
		return
	}
	// Unknown path: hand back the SPA shell for client-side routing.
	s.serveIndex(w)
}

func (s *SPA) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(s.index)
}
