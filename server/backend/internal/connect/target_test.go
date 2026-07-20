package connect

import "testing"

func TestValidateTargetDenies(t *testing.T) {
	deny := []string{
		"169.254.169.254:80", // cloud metadata endpoint — the headline SSRF target
		"169.254.1.1:22",     // link-local unicast
		"0.0.0.0:80",         // unspecified v4
		"[::]:80",            // unspecified v6
		"224.0.0.1:80",       // multicast
		"[ff02::1]:80",       // link-local multicast
	}
	for _, tgt := range deny {
		if err := validateTarget(tgt); err == nil {
			t.Errorf("expected %q to be denied", tgt)
		}
	}
}

func TestValidateTargetAllows(t *testing.T) {
	// Loopback and private ranges are the *purpose* of /connect (reach the
	// device's own services and LAN); hostnames resolve on the device.
	allow := []string{
		"127.0.0.1:22",
		"10.0.0.5:5432",
		"192.168.1.10:22",
		"example.com:443",
		"db.internal:5432",
	}
	for _, tgt := range allow {
		if err := validateTarget(tgt); err != nil {
			t.Errorf("expected %q to be allowed, got %v", tgt, err)
		}
	}
}

func TestValidateTargetMalformed(t *testing.T) {
	for _, tgt := range []string{"", "noport", "host:", ":22"} {
		if err := validateTarget(tgt); err == nil {
			t.Errorf("expected %q to be rejected as malformed", tgt)
		}
	}
}
