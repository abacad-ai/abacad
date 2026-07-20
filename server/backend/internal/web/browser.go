package web

import (
	_ "embed"
	"net/http"
)

// browserClientHTML is the self-contained browser-device client served at /b.
// A user opens /b#<device-token>; the page dials the /device WebSocket and acts
// as a device — screenshot (DOM tree + pixels), click/scroll/input_text, and
// execute (JS in its content iframe). It has no build step; it is embedded and
// served verbatim.
//
//go:embed browser.html
var browserClientHTML []byte

// BrowserClient serves the browser-device client page. The device token is
// carried in the URL fragment by the caller, so it never reaches the server —
// this handler needs no auth and returns the same static page to everyone.
func BrowserClient() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(browserClientHTML)
	})
}
