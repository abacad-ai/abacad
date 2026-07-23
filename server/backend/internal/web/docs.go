package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// The public documentation site (abacad.ai/docs), built by the Astro project in
// server/docs-site and copied to docs-dist before `go build` — the same
// embed-a-static-tree pattern as the dashboard SPA (spa.go). The `all:` prefix
// is required so Astro's hashed asset dir (_astro/) isn't dropped by embed's
// default exclusion of names beginning with "_".
//
//go:embed all:docs-dist
var docsDistFS embed.FS

// Docs serves the static docs site under /docs. The Astro build is configured
// with base:'/docs', so its internal asset and link URLs already carry the /docs
// prefix; StripPrefix removes it before the file server resolves against the
// embedded tree. Mount with the trailing-slash subtree pattern so Go 1.22's
// ServeMux precedence lets it dominate the SPA catch-all, and so `/docs` (no
// slash) auto-redirects to `/docs/`:
//
//	mux.Handle("GET /docs/", web.Docs())
func Docs() http.Handler {
	sub, err := fs.Sub(docsDistFS, "docs-dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/docs/", http.FileServer(http.FS(sub)))
}
