package web

import (
	_ "embed"
	"net/http"
)

// Privacy policy and terms of service, served as self-contained static HTML.
// They are public, need no JavaScript, and require no auth — Google's OAuth
// consent screen requires reachable privacy-policy and terms URLs, and these
// pages satisfy that while also being the human-readable source of truth.
//
//go:embed privacy.html
var privacyHTML []byte

//go:embed terms.html
var termsHTML []byte

// PrivacyPolicy serves the privacy policy at a stable public URL (/privacy).
func PrivacyPolicy() http.Handler { return staticHTML(privacyHTML) }

// TermsOfService serves the terms of service at a stable public URL (/terms).
func TermsOfService() http.Handler { return staticHTML(termsHTML) }

func staticHTML(body []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(body)
	})
}
