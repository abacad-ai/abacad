package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"abacad/internal/auth"
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

// quotaLimiter caps how often a key may perform an action that has no notion of
// failure. loginLimiter is the wrong shape for this: it counts CONSECUTIVE
// FAILURES, but every device self-registration succeeds — the abuse is the
// volume, not the outcome.
//
// It is a token bucket: burst tokens available immediately, refilling at
// rate-per-window. State is in-memory and resets on restart, which is acceptable
// for the same reason loginLimiter's is — the goal is to make bulk automation
// expensive, and a restart doesn't hand an attacker a durable window. The
// database-backed caps (live registrations per IP, global total) are what bound
// the damage across restarts.
type quotaLimiter struct {
	mu     sync.Mutex
	byKey  map[string]*bucket
	burst  float64       // tokens available at once
	per    time.Duration // time to refill one full burst
	maxIdl time.Duration // drop bucket state after this long untouched
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newQuotaLimiter(burst int, per time.Duration) *quotaLimiter {
	return &quotaLimiter{
		byKey:  make(map[string]*bucket),
		burst:  float64(burst),
		per:    per,
		maxIdl: 2 * per,
	}
}

// take consumes one token for key. It reports whether the action is allowed and,
// if not, how long until a token is available (for Retry-After).
func (q *quotaLimiter) take(key string, now time.Time) (ok bool, retryAfter time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sweep(now)

	b := q.byKey[key]
	if b == nil {
		b = &bucket{tokens: q.burst, last: now}
		q.byKey[key] = b
	}
	// Refill for elapsed time, capped at burst.
	refill := now.Sub(b.last).Seconds() / q.per.Seconds() * q.burst
	b.tokens = min(q.burst, b.tokens+refill)
	b.last = now

	if b.tokens < 1 {
		need := (1 - b.tokens) / q.burst * q.per.Seconds()
		return false, time.Duration(need * float64(time.Second))
	}
	b.tokens--
	return true, 0
}

// sweep drops idle buckets so the map can't grow without bound. Mirrors
// loginLimiter.sweep: only does work once the map is non-trivially large.
func (q *quotaLimiter) sweep(now time.Time) {
	if len(q.byKey) < 1024 {
		return
	}
	for k, b := range q.byKey {
		if now.Sub(b.last) > q.maxIdl {
			delete(q.byKey, k)
		}
	}
}

// writeThrottled rejects a request that exceeded its quota, advertising when to
// come back so a well-behaved client backs off instead of hammering.
func writeThrottled(w http.ResponseWriter, retryAfter time.Duration, msg string) {
	secs := int(retryAfter.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeErr(w, http.StatusTooManyRequests, msg)
}

// clientIP is the throttle/attribution key for a request. It lives in auth so the
// device and connect handlers — which record the same address into the activity
// trail but never import this package — resolve it identically. See isHTTPS for
// the reverse-proxy assumption it shares.
func clientIP(r *http.Request) string { return auth.ClientIP(r) }
