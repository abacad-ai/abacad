package gateway

import (
	"sync"
	"time"
)

// failLimiter is a small in-memory guard for raw VNC token attempts. It is not
// a security boundary (an attacker can rotate source addresses on a LAN), but it
// stops a single client from hammering an 8-character token and keeps the logs
// readable.
type failLimiter struct {
	mu sync.Mutex
	m  map[string]*failEntry
}

type failEntry struct {
	count int
	last  time.Time
	until time.Time
}

func newFailLimiter() *failLimiter { return &failLimiter{m: map[string]*failEntry{}} }

func (l *failLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.m[key]
	if e == nil {
		return true
	}
	if now.Before(e.until) {
		return false
	}
	if now.Sub(e.last) > time.Minute {
		delete(l.m, key)
	}
	return true
}

func (l *failLimiter) fail(key string) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.m[key]
	if e == nil {
		e = &failEntry{}
		l.m[key] = e
	}
	if now.Sub(e.last) > time.Minute {
		e.count = 0
	}
	e.last = now
	e.count++
	if e.count >= 5 {
		e.until = now.Add(time.Minute)
		e.count = 0
	}
}

func (l *failLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.m, key)
	l.mu.Unlock()
}
