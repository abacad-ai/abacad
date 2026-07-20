package connect

import (
	"fmt"
	"net"
	"net/netip"
)

// validateTarget rejects tunnel targets that have no legitimate use and clear
// SSRF value: the cloud metadata endpoint (169.254.169.254) and other
// link-local, unspecified, or multicast addresses. An agent must never be able
// to point a device's dial at 169.254.169.254 to lift IAM credentials.
//
// Loopback and private ranges are intentionally ALLOWED — reaching the device's
// own services (a DB, sshd, a dev server) and the device's LAN is the whole
// purpose of /connect, and the SSH jump likewise targets the device's own
// 127.0.0.1:22. So this is not a default-deny of internal ranges; it is a
// targeted deny of the addresses that are never a real tunnel target.
//
// This check is best-effort by design: only a literal-IP target can be judged
// here, because the DEVICE performs the DNS resolution and the actual dial on
// its own network. Authoritative, resolution-aware policy is enforced
// device-side (see docs/trust.md); this server-side guard blocks the obvious
// literal-IP SSRF and rejects malformed targets early.
func validateTarget(target string) error {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("target must be host:port")
	}
	if host == "" || port == "" {
		return fmt.Errorf("target must be host:port")
	}
	// Hostnames resolve on the device; we can only judge literal IPs here.
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	switch {
	case addr.IsLinkLocalUnicast(), // 169.254.0.0/16 (incl. metadata), fe80::/10
		addr.IsLinkLocalMulticast(),
		addr.IsMulticast(),
		addr.IsUnspecified(): // 0.0.0.0, ::
		return fmt.Errorf("target %s is not an allowed address", host)
	}
	return nil
}
