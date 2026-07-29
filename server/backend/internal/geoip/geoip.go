// Package geoip resolves a client IP to a coarse location for the activity
// trail, so "this key ran a command from an address I don't recognise" can be
// read as "…from a country I've never worked from".
//
// It wraps a MaxMind GeoLite2/GeoIP2 City database via geoip2-golang. The
// database is NOT shipped with abacad: MaxMind's terms don't allow redistributing
// it, and it goes stale (they republish weekly), so operators supply their own
// and point -geoip-db at it. With no database configured every lookup returns
// empty and the trail simply carries no location — the feature is additive, never
// load-bearing.
//
// # Accuracy
//
// Country is generally reliable. City is not: it is frequently the wrong city in
// the right region, and for mobile carriers, VPNs and CGNAT it can be off by a
// country. Treat city as a hint to eyeball, never as evidence — which is why
// callers surface country first.
package geoip

import (
	"net/netip"
	"sync"

	geoip2 "github.com/oschwald/geoip2-golang/v2"
)

// Locator resolves addresses to (country, city). Safe for concurrent use, and a
// nil *Locator is a working no-op so callers never have to branch on "is geo
// configured" — the zero result is the same one a private address produces.
type Locator struct {
	mu     sync.RWMutex
	reader *geoip2.Reader
}

// Open loads a MaxMind City database. An error means the file is missing or not a
// City database; the caller decides whether that is fatal (it need not be —
// running without geo is a supported configuration).
func Open(path string) (*Locator, error) {
	r, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}
	return &Locator{reader: r}, nil
}

// Close releases the database. Safe on a nil Locator and safe to call twice.
func (l *Locator) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reader == nil {
		return nil
	}
	err := l.reader.Close()
	l.reader = nil
	return err
}

// Lookup resolves a textual IP to an ISO 3166-1 alpha-2 country code and an
// English city name. Either may be empty — a database can know the country and
// not the city — and both are empty when geo is unconfigured, the address is
// unparseable, or it is one the database can say nothing useful about.
//
// The ISO code is stored rather than a country name because it is stable, compact
// and exact to filter on; rendering it as a name or flag is the UI's job.
func (l *Locator) Lookup(ip string) (country, city string) {
	if l == nil {
		return "", ""
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", ""
	}
	// A private, loopback or link-local address is never in the database, and on a
	// self-hosted relay most traffic is exactly that. Skipping them avoids a
	// pointless lookup per row and keeps the column empty rather than misleading.
	if !isGlobalUnicast(addr) {
		return "", ""
	}

	l.mu.RLock()
	r := l.reader
	l.mu.RUnlock()
	if r == nil {
		return "", ""
	}

	rec, err := r.City(addr.Unmap())
	if err != nil || rec == nil {
		return "", ""
	}
	if rec.Country.HasData() {
		country = rec.Country.ISOCode
	}
	if rec.City.HasData() {
		city = rec.City.Names.English
	}
	return country, city
}

// isGlobalUnicast reports whether addr is a public address worth geolocating.
// netip's own predicates cover loopback/link-local/multicast/unspecified;
// RFC 1918 and unique-local ranges are checked explicitly because netip has no
// "is private" helper.
func isGlobalUnicast(addr netip.Addr) bool {
	if !addr.IsValid() || addr.IsLoopback() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return false
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.Is4() {
		b := addr.As4()
		switch {
		case b[0] == 10:
			return false
		case b[0] == 172 && b[1]&0xf0 == 16:
			return false
		case b[0] == 192 && b[1] == 168:
			return false
		case b[0] == 100 && b[1]&0xc0 == 64: // 100.64/10 CGNAT
			return false
		}
		return true
	}
	// fc00::/7 unique-local.
	return addr.As16()[0]&0xfe != 0xfc
}
