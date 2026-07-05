package auth

import (
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
