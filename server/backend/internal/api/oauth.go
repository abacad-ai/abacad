// OAuth "Sign in with Google". A confidential web-app flow: the browser is
// redirected to Google, comes back to /api/auth/google/callback with a code, and
// the server exchanges the code (using its client secret, over a direct TLS
// connection to Google) for tokens, then reads the verified identity from
// Google's userinfo endpoint. On success it mints the same session cookie the
// password path does — everything downstream is identical.
//
// No id_token JWT parsing: the code is exchanged server-to-server against
// accounts.google.com, and the identity is fetched from userinfo with the
// resulting access token, so there is no third-party assertion to verify locally.
package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"abacad/internal/activity"
	"abacad/internal/store"
)

// Google OAuth 2.0 / OIDC endpoints.
const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

// oauthStateCookie carries the anti-CSRF state across the redirect to Google and
// back. Short-lived and scoped to the callback path.
const oauthStateCookie = "abacad_oauth_state"

// oauthClient is used for the server-to-server token and userinfo calls; a tight
// timeout keeps a slow Google from tying up the request.
var oauthClient = &http.Client{Timeout: 10 * time.Second}

// GoogleEnabled reports whether the Google sign-in flow is configured.
func (a *API) GoogleEnabled() bool {
	return a.GoogleClientID != "" && a.GoogleClientSecret != ""
}

// authConfig tells the frontend which optional sign-in methods are available, so
// it only renders a "Continue with Google" button when the server can honor it.
func (a *API) authConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"google": a.GoogleEnabled()})
}

// googleRedirectURI is the callback URL registered with Google. Configured value
// wins; otherwise it is derived from the request so a single-host deployment
// needs no extra config (the derived URL must still be registered in Google).
func (a *API) googleRedirectURI(r *http.Request) string {
	if a.GoogleRedirectURL != "" {
		return a.GoogleRedirectURL
	}
	return httpURL(r, "/api/auth/google/callback")
}

// googleStart kicks off the flow: set a random state cookie and redirect to
// Google's consent screen.
func (a *API) googleStart(w http.ResponseWriter, r *http.Request) {
	if !a.GoogleEnabled() {
		writeErr(w, http.StatusNotFound, "google sign-in is not configured")
		return
	}
	state, err := randState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not start sign-in")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/api/auth/google",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // sent on the top-level redirect back from Google
		Secure:   isHTTPS(r),
		MaxAge:   600, // 10 minutes to complete consent
	})
	q := url.Values{
		"client_id":     {a.GoogleClientID},
		"redirect_uri":  {a.googleRedirectURI(r)},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		// Ask for a fresh account chooser rather than silently reusing a session.
		"prompt": {"select_account"},
	}
	http.Redirect(w, r, googleAuthURL+"?"+q.Encode(), http.StatusFound)
}

// googleCallback completes the flow: verify state, exchange the code, read the
// verified identity, find-or-create the account, and start a session.
func (a *API) googleCallback(w http.ResponseWriter, r *http.Request) {
	if !a.GoogleEnabled() {
		writeErr(w, http.StatusNotFound, "google sign-in is not configured")
		return
	}
	// Clear the state cookie regardless of outcome.
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/api/auth/google", MaxAge: -1})

	q := r.URL.Query()
	if e := q.Get("error"); e != "" { // user denied consent, etc.
		a.oauthFail(w, r, "google sign-in was cancelled")
		return
	}
	state := q.Get("state")
	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || state == "" || subtleMismatch(cookie.Value, state) {
		a.oauthFail(w, r, "sign-in expired or was tampered with; please try again")
		return
	}
	code := q.Get("code")
	if code == "" {
		a.oauthFail(w, r, "google did not return an authorization code")
		return
	}

	id, err := a.googleIdentity(r, code)
	if err != nil {
		a.oauthFail(w, r, "could not verify your Google account")
		return
	}
	if id.Sub == "" || id.Email == "" || !id.EmailVerified {
		a.oauthFail(w, r, "your Google account has no verified email")
		return
	}

	email := strings.TrimSpace(strings.ToLower(id.Email))
	acc, created, err := a.Store.LinkGoogleAccount(id.Sub, email)
	if err != nil {
		a.oauthFail(w, r, "could not sign you in")
		return
	}
	a.startSession(w, r, acc)
	if created {
		a.record(acc.ID, store.Activity{Kind: activity.KindRegister, Source: "google", Detail: acc.Email})
	}
	a.record(acc.ID, store.Activity{Kind: activity.KindLogin, Source: "google"})
	http.Redirect(w, r, "/", http.StatusFound)
}

// googleIdentity exchanges the authorization code for tokens and reads the
// verified profile from Google's userinfo endpoint.
func (a *API) googleIdentity(r *http.Request, code string) (googleUser, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {a.GoogleClientID},
		"client_secret": {a.GoogleClientSecret},
		"redirect_uri":  {a.googleRedirectURI(r)},
		"grant_type":    {"authorization_code"},
	}
	resp, err := oauthClient.PostForm(googleTokenURL, form)
	if err != nil {
		return googleUser{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleUser{}, errors.New("google token exchange failed")
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		return googleUser{}, errors.New("google token response malformed")
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, googleUserinfoURL, nil)
	if err != nil {
		return googleUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	ui, err := oauthClient.Do(req)
	if err != nil {
		return googleUser{}, err
	}
	defer ui.Body.Close()
	if ui.StatusCode != http.StatusOK {
		return googleUser{}, errors.New("google userinfo failed")
	}
	var user googleUser
	if err := json.NewDecoder(ui.Body).Decode(&user); err != nil {
		return googleUser{}, err
	}
	return user, nil
}

// googleUser is the subset of Google's userinfo we consume.
type googleUser struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// oauthFail sends the browser back to the login page with a human-readable
// message (query param the AuthPage surfaces).
func (a *API) oauthFail(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/login?error="+url.QueryEscape(msg), http.StatusFound)
}

// randState returns a URL-safe high-entropy state value.
func randState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// subtleMismatch reports whether two strings differ, in constant time w.r.t. the
// compared bytes (the state is not a long-lived secret, but a constant-time
// compare costs nothing and avoids a length/early-exit oracle).
func subtleMismatch(a, b string) bool {
	if len(a) != len(b) {
		return true
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v != 0
}
