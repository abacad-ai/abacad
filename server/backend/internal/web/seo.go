package web

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// og-image.png is the social-unfurl card referenced by index.html's og:image /
// twitter:image. Served from the web package (not the SPA dist) so it doesn't
// require a frontend rebuild; rendered from public/og-image.svg at 1200×630.
//
//go:embed og-image.png
var ogImagePNG []byte

// OGImage serves the social card at a stable public URL (/og-image.png).
func OGImage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(ogImagePNG)
	})
}

// SEO surface: robots.txt and sitemap.xml. Kept alongside the other static web
// handlers (legal.go, spa.go). Both are public, need no auth, and are served on
// the apex/marketing origin. Content is templated with the configured base
// domain so self-host / dev deployments advertise the right URLs.

// publicPaths are the crawlable, indexable marketing routes. The docs site's own
// pages are enumerated by its Starlight-generated /docs/sitemap-index.xml, which
// robots.txt advertises separately; here we just include the docs root.
var publicPaths = []string{"/", "/docs/", "/downloads", "/privacy", "/terms"}

// WriteRobots writes robots.txt. allowIndex is true on the apex/marketing origin
// and false on per-device subdomains (<id>.abacad.ai) — device pages carry no
// public content and must never be indexed, so those hosts disallow everything.
func WriteRobots(w http.ResponseWriter, allowIndex bool, baseDomain string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	if !allowIndex {
		// Per-device host: keep it entirely out of every index.
		_, _ = io.WriteString(w, "User-agent: *\nDisallow: /\n")
		return
	}

	var b strings.Builder
	b.WriteString("User-agent: *\n")
	b.WriteString("Allow: /\n")
	// Keep the API and the authenticated dashboard routes out of the index; they
	// carry no public content and would only dilute ranking.
	for _, p := range []string{"/api/", "/devices/", "/activities", "/access", "/settings", "/pair"} {
		fmt.Fprintf(&b, "Disallow: %s\n", p)
	}
	fmt.Fprintf(&b, "\nSitemap: https://%s/sitemap.xml\n", baseDomain)
	fmt.Fprintf(&b, "Sitemap: https://%s/docs/sitemap-index.xml\n", baseDomain)
	_, _ = io.WriteString(w, b.String())
}

// Sitemap returns the sitemap.xml handler for the apex origin.
func Sitemap(baseDomain string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
		b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
		for _, p := range publicPaths {
			fmt.Fprintf(&b, "  <url><loc>https://%s%s</loc></url>\n", baseDomain, p)
		}
		b.WriteString("</urlset>\n")
		_, _ = io.WriteString(w, b.String())
	})
}
