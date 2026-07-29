package auth

import (
	"net"
	"net/http"
	"strings"
)

// SessionCookie is the name of the dashboard session cookie.
const SessionCookie = "abacad_session"

// BearerToken extracts the token from an "Authorization: Bearer <token>" header,
// or "" if absent/malformed.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// SessionID reads the session cookie value, or "" if absent.
func SessionID(r *http.Request) string {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// SessionFingerprint derives a stable, non-reversible handle for a session, for
// use as an activity-trail actor id.
//
// The raw session id must never go in the trail: it is the live cookie value, and
// the trail is served back over GET /api/activities — writing it there would hand
// out working session credentials to anything that can read an account's history.
// A truncated hash still answers "were these two actions the same browser
// session?", which is the question the trail needs. Empty in, empty out, so
// pre-login events (a failed sign-in) get no actor id rather than a hash of "".
func SessionFingerprint(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return "sess_" + HashToken(sessionID)[:12]
}

// ClientIP is the throttle/attribution key for a request: the left-most
// X-Forwarded-For hop when behind a reverse proxy (this server is designed to run
// behind one), else the direct peer address.
//
// The header is trusted unconditionally, so the proxy MUST overwrite
// X-Forwarded-For rather than append to it — otherwise a client can pick the
// address that lands in the rate limiter and the activity trail. Deployments that
// expose this server directly get a spoofable value here; see the self-hosting
// guide.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
