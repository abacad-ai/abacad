package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginLimiterLocksAndRecovers(t *testing.T) {
	l := newLoginLimiter()
	now := time.Now()
	const key = "1.2.3.4"

	for i := 0; i < l.maxFails; i++ {
		if ok, _ := l.allowed(key, now); !ok {
			t.Fatalf("locked too early at attempt %d", i)
		}
		l.recordFail(key, now)
	}

	ok, retry := l.allowed(key, now)
	if ok {
		t.Fatalf("expected lockout after %d fails", l.maxFails)
	}
	if retry <= 0 {
		t.Fatalf("expected a positive retry-after, got %v", retry)
	}

	if ok, _ := l.allowed(key, now.Add(l.lockout+time.Second)); !ok {
		t.Fatalf("expected recovery after the lockout window elapses")
	}
}

func TestLoginLimiterPerIP(t *testing.T) {
	l := newLoginLimiter()
	now := time.Now()
	for i := 0; i < l.maxFails; i++ {
		l.recordFail("1.2.3.4", now)
	}
	if ok, _ := l.allowed("1.2.3.4", now); ok {
		t.Fatal("attacker IP should be locked")
	}
	if ok, _ := l.allowed("5.6.7.8", now); !ok {
		t.Fatal("a different IP must not be affected (no victim lockout)")
	}
}

func TestLoginLimiterResetOnSuccess(t *testing.T) {
	l := newLoginLimiter()
	now := time.Now()
	const key = "1.2.3.4"
	for i := 0; i < l.maxFails; i++ {
		l.recordFail(key, now)
	}
	l.reset(key)
	if ok, _ := l.allowed(key, now); !ok {
		t.Fatal("expected allowed after a successful-login reset")
	}
}

func TestClientIPForwardedFor(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("clientIP = %q, want left-most XFF hop", got)
	}

	r2 := httptest.NewRequest("POST", "/api/auth/login", nil)
	r2.RemoteAddr = "198.51.100.7:4444"
	if got := clientIP(r2); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want RemoteAddr host", got)
	}
}
