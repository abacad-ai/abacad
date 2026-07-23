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

// LLMsTxt serves /llms.txt — a curated, agent-facing index of the public docs
// (the llmstxt.org convention). It gives an LLM a compact, high-signal map of
// what abacad is and where to read more, in plain markdown, without crawling the
// HTML. Templated with the base domain so links resolve on any deployment.
func LLMsTxt(baseDomain string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")

		base := "https://" + baseDomain
		fmt.Fprintf(w, `# abacad

> A device interface for agents. Connect a phone, laptop, or browser as a device and let a coding agent see the screen and act on it — one step at a time, with a human approving.

abacad connects a real device (an Android phone, a Mac, a Linux box, or a browser tab) to an AI agent over one MCP endpoint. The device dials out and holds a relay connection, so it works through NAT with no inbound port. The agent drives it with a uniform tool contract — a screenshot plus the accessibility tree per step, tap/click/type/swipe, run_command, and push_file/pull_file — staying on the highest (most semantic) rung the task allows. A human supervises and approves sensitive actions. It is agent-native: one screenshot + tree per step, not a live video mirror.

## Docs

- [What abacad is](%s/docs/): the product, the control-surface ladder, and the honest platform matrix.
- [Tool reference](%s/docs/reference/tools/): every agent-facing operation (screenshot, tap, swipe, type, run_command, execute, push_file/pull_file), with per-platform status.
- [Screen recording](%s/docs/reference/screen-recording/): the screen_recording tool — a high-quality file artifact plus an optional live view.
- [Transport](%s/docs/reference/transport/): the JSON control plane vs the HTTP data plane (/blobs), split by message type.
- [Reading status markers](%s/docs/reference/status-markers/): what the shipped / built / envisioned markers mean.
- [SSH access](%s/docs/guides/ssh/): reach a device's own sshd behind NAT with stock ssh and nothing installed.
- [Running a phone hands-off](%s/docs/guides/running-hands-off/): keep an Android device reachable long-term on a charger.
- [Security](%s/docs/security/): the two-plane trust model, the controls in place today, and the honest limit (prompt injection).

## Full text

- [All docs as a single file](%s/docs/llms-full.txt): the complete documentation concatenated for one-shot ingestion.

## Notes

- Capabilities are marked per platform: ✅ shipped, 🟡 client built but unproven, 🔮 envisioned. Read the marker for the platform you care about, not the row as a whole.
- Android is the furthest along; macOS and Linux clients are built; Windows and iOS are planned; a browser tab can act as a device with no install.
`, base, base, base, base, base, base, base, base, base)
	})
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
