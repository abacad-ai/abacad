// Package web serves the built dashboard SPA from an embedded copy of the Vite
// dist directory. The build script copies frontend/dist here before `go build`
// (embed can't reach outside the module). Unknown paths fall back to index.html
// so client-side routes (e.g. /settings) work on hard refresh.
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

// New builds the SPA handler from the embedded dist.
func New() (*SPA, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
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
// placeholder. Lets the server log a hint in dev.
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
