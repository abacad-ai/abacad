package relay

import "testing"

// TestHubKick closes a live connection and reports whether one existed.
func TestHubKick(t *testing.T) {
	h := NewHub()

	// No connection for the id: Kick is a no-op returning false.
	if h.Kick("absent") {
		t.Fatal("Kick on an unknown id should return false")
	}

	c := newTestConn("d1")
	h.Register(c)
	if !h.Kick("d1") {
		t.Fatal("Kick should return true for a live device")
	}
	select {
	case <-c.closed:
	default:
		t.Fatal("kicked connection should be closed")
	}
}
