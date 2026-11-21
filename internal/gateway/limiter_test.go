package gateway

import "testing"

func TestFailLimiter(t *testing.T) {
	l := newFailLimiter()
	key := "device|192.0.2.10:1234"
	if !l.allow(key) {
		t.Fatal("first attempt should be allowed")
	}
	for i := 0; i < 5; i++ {
		l.fail(key)
	}
	if l.allow(key) {
		t.Fatal("after five failures the key should be blocked")
	}
	l.reset(key)
	if !l.allow(key) {
		t.Fatal("reset should clear the block")
	}
}
