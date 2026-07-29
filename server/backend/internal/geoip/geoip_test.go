package geoip

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture is MaxMind's own synthetic City database, published in the MaxMind-DB
// repository (dual MIT/Apache-2.0) for exactly this purpose. It is NOT a real
// GeoLite2 database and contains only a handful of invented ranges — enough to
// prove the wiring, small enough to keep in the tree.
const fixture = "testdata/GeoIP2-City-Test.mmdb"

func openFixture(t *testing.T) *Locator {
	t.Helper()
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("no test database at %s: %v", fixture, err)
	}
	l, err := Open(fixture)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestLookup(t *testing.T) {
	l := openFixture(t)
	cases := []struct {
		ip, country, city string
	}{
		{"81.2.69.142", "GB", "London"},
		{"2.125.160.216", "GB", "Boxford"},
		{"89.160.20.112", "SE", "Linköping"}, // non-ASCII city name survives intact
		{"216.160.83.56", "US", "Milton"},
		// Present in no range: a miss is empty, never a guess.
		{"8.8.8.8", "", ""},
		// Private, loopback and CGNAT are skipped before the database is consulted;
		// on a self-hosted relay most traffic looks like this.
		{"10.0.0.1", "", ""},
		{"192.168.1.5", "", ""},
		{"172.16.0.9", "", ""},
		{"100.64.0.1", "", ""},
		{"127.0.0.1", "", ""},
		{"::1", "", ""},
		{"fd00::1", "", ""},
		// Garbage in, empty out — the IP column is free-form text upstream.
		{"", "", ""},
		{"not-an-ip", "", ""},
		{"203.0.113.9:443", "", ""}, // host:port, not an address
	}
	for _, c := range cases {
		country, city := l.Lookup(c.ip)
		if country != c.country || city != c.city {
			t.Errorf("Lookup(%q) = (%q, %q), want (%q, %q)", c.ip, country, city, c.country, c.city)
		}
	}
}

// An IPv4-mapped IPv6 address must resolve like its IPv4 form: Go yields these
// from dual-stack listeners, so without unmapping every such client would silently
// lose its location.
func TestLookupUnmapsIPv4In6(t *testing.T) {
	l := openFixture(t)
	country, city := l.Lookup("::ffff:81.2.69.142")
	if country != "GB" || city != "London" {
		t.Errorf("got (%q, %q), want (GB, London)", country, city)
	}
}

// Geo is optional. A nil Locator is the "not configured" state and must behave
// like a miss rather than panicking, since it is consulted on every trail write.
func TestNilLocatorIsNoOp(t *testing.T) {
	var l *Locator
	if country, city := l.Lookup("81.2.69.142"); country != "" || city != "" {
		t.Errorf("nil locator returned (%q, %q), want empty", country, city)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close on nil locator: %v", err)
	}
}

func TestClosedLocatorIsNoOp(t *testing.T) {
	l := openFixture(t)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Double close is a no-op, and lookups after close return empty rather than
	// touching a freed reader.
	if err := l.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
	if country, city := l.Lookup("81.2.69.142"); country != "" || city != "" {
		t.Errorf("closed locator returned (%q, %q), want empty", country, city)
	}
}

func TestOpenMissingFile(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "nope.mmdb")); err == nil {
		t.Fatal("want an error for a missing database")
	}
}
