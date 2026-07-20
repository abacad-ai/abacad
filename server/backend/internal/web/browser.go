package web

import (
	_ "embed"
	"net/http"
)

// browserClientHTML is the self-contained browser-device client. It is served at
// the root of a device's own subdomain (<device-id>.abacad.ai); the page dials
// the /device WebSocket same-origin and acts as a device — screenshot (DOM tree
// + pixels), click/scroll/input_text, and execute (JS in its content iframe). It
// has no build step; it is embedded and served verbatim.
//
//go:embed browser.html
var browserClientHTML []byte

// BrowserClient serves the browser-device client page. Authentication happens on
// the /device WebSocket (by the device id in the request Host), not here — the
// page is the same static asset for every device, so this handler needs no auth.
func BrowserClient() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(browserClientHTML)
	})
}
