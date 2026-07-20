package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginLimiter throttles password attempts to blunt brute force. It is keyed by
// client IP, NOT by account, on purpose: locking an account by its email would
// let anyone lock a victim out (a denial-of-service). Per-IP throttling slows a
// real attacker without giving them a lockout weapon. bcrypt already makes each
// individual guess expensive; this caps the rate on top.
//
// State is in-memory (resets on restart), which is fine at single-host scale:
// the goal is to defeat online guessing, and a restart doesn't hand an attacker
// a usable window.
type loginLimiter struct {
	mu       sync.Mutex
	byKey    map[string]*failCounter
	maxFails int           // consecutive fails within window before lockout
	window   time.Duration // fails older than this don't count toward lockout
	lockout  time.Duration // how long a locked key stays locked
}

type failCounter struct {
	fails       int
	first       time.Time
	lockedUntil time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		byKey:    make(map[string]*failCounter),
		maxFails: 10,
		window:   15 * time.Minute,
		lockout:  15 * time.Minute,
	}
}

// allowed reports whether key may attempt a login now. If not, retryAfter is how
// long until it may try again.
func (l *loginLimiter) allowed(key string, now time.Time) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.byKey[key]
	if c == nil {
		return true, 0
	}
	if now.Before(c.lockedUntil) {
		return false, c.lockedUntil.Sub(now)
	}
	return true, 0
}

// recordFail notes a failed attempt for key and locks it once maxFails is reached
// within the window.
func (l *loginLimiter) recordFail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	c := l.byKey[key]
	if c == nil || now.Sub(c.first) > l.window {
		c = &failCounter{first: now}
		l.byKey[key] = c
	}
	c.fails++
	if c.fails >= l.maxFails {
		c.lockedUntil = now.Add(l.lockout)
		c.fails = 0
		c.first = now
	}
}

// reset clears a key's failure state after a successful login.
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byKey, key)
}

// sweep drops stale entries so the map can't grow without bound. Called under the
// lock from recordFail; only runs work once the map is non-trivially large.
func (l *loginLimiter) sweep(now time.Time) {
	if len(l.byKey) < 1024 {
		return
	}
	for k, c := range l.byKey {
		if now.After(c.lockedUntil) && now.Sub(c.first) > l.window {
			delete(l.byKey, k)
		}
	}
}

// clientIP is the throttle/attribution key for a request: the left-most
// X-Forwarded-For hop when behind a reverse proxy (this server is designed to run
// behind one — see isHTTPS), else the direct peer address.
func clientIP(r *http.Request) string {
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
