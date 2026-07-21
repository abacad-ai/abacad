package agent

import "testing"

func TestIsBlockedTargetHost(t *testing.T) {
	blocked := []string{
		"169.254.169.254", // cloud metadata
		"169.254.0.1",     // link-local
		"0.0.0.0",         // unspecified
		"224.0.0.1",       // multicast
		"239.255.255.250", // multicast (SSDP)
		"::",              // IPv6 unspecified
		"fe80::1",         // IPv6 link-local
		"ff02::1",         // IPv6 multicast
	}
	for _, h := range blocked {
		if !isBlockedTargetHost(h) {
			t.Errorf("expected %q blocked", h)
		}
	}
	allowed := []string{
		"127.0.0.1",       // loopback
		"10.0.0.5",        // private
		"192.168.1.10",    // private
		"172.16.0.1",      // private
		"example.com",     // hostname
		"224.example.com", // hostname that merely starts like multicast
		"8.8.8.8",         // public
		"169.example.com", // hostname
	}
	for _, h := range allowed {
		if isBlockedTargetHost(h) {
			t.Errorf("expected %q allowed", h)
		}
	}
}

func TestEncodeFrameRoundTrip(t *testing.T) {
	payload := []byte("host:22")
	f := encodeFrame(frameOpen, 0x0102030405060708, payload)
	if len(f) != streamHeaderLen+len(payload) {
		t.Fatalf("frame len = %d, want %d", len(f), streamHeaderLen+len(payload))
	}
	if f[0] != frameOpen {
		t.Errorf("type = %d, want %d", f[0], frameOpen)
	}
	if string(f[streamHeaderLen:]) != string(payload) {
		t.Errorf("payload = %q, want %q", f[streamHeaderLen:], payload)
	}
}
