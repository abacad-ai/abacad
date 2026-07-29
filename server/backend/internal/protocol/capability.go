package protocol

import (
	"sort"
	"strings"
)

// Capability is one switchable interface a device exposes. Every protocol method
// is a capability under the same name, plus the three below that ride the binary
// tunnel lane or are server-initiated and so have no Method of their own.
//
// This is *scope*, not approval: a capability is a static property of the device,
// declared once by its owner, carrying no judgement about whether any particular
// action is safe. Deciding that needs the task's intent, which lives in the agent
// (see docs/trust.md). The device says what it exposes; it does not referee.
type Capability string

const (
	// CapTunnel is a raw TCP tunnel to a device-reachable host:port (/connect).
	//
	// It is a SUPERSET of nearly every other capability, not a peer of them:
	// loopback targets are deliberately allowed (see connect.validateTarget), so
	// a tunnel reaches the device's own sshd, its ADB port, a Chrome DevTools
	// port (JS evaluation and screen capture), an X server, a local database.
	// Granting it is close to granting everything; the UI must say so.
	CapTunnel Capability = "tunnel"

	// CapSSH is the jump host bridging a stock ssh client to the device's own
	// 127.0.0.1:22. Implied by CapTunnel, which can dial that port directly.
	CapSSH Capability = "ssh"

	// CapVNC is the live view channel. It is NOT read-only: the RFB pipe is
	// bidirectional (vnc.pipe copies viewer->device too), and viewer frames carry
	// PointerEvent and KeyEvent. Treat it as observation *and* input.
	CapVNC Capability = "vnc"
)

// Capabilities is every capability a device can be configured to expose, in a
// stable order: the protocol methods first (MCP tool order), then the three that
// have no method. This is the source of truth for validating a capability set.
var Capabilities = func() []Capability {
	caps := make([]Capability, 0, len(Methods)+3)
	for _, m := range Methods {
		caps = append(caps, Capability(m))
	}
	return append(caps, CapTunnel, CapSSH, CapVNC)
}()

// CapabilitySet is a device's configured capabilities. The zero value allows
// nothing, which is the safe default for a value that failed to load: a gate
// that fails open is not a gate.
type CapabilitySet struct {
	all bool
	set map[Capability]bool
}

// AllCapabilities returns the wildcard set: everything, including capabilities
// added in future versions. This is what an enrolled device gets by default —
// enrollment is the authorization (docs/trust.md), so the config is opt-in
// hardening rather than a second gate a user must pass to get a working device.
func AllCapabilities() CapabilitySet { return CapabilitySet{all: true} }

// NewCapabilitySet returns a set allowing exactly caps.
func NewCapabilitySet(caps ...Capability) CapabilitySet {
	s := CapabilitySet{set: make(map[Capability]bool, len(caps))}
	for _, c := range caps {
		s.set[c] = true
	}
	return s
}

// ParseCapabilities decodes the devices.capabilities column, mirroring the
// api_keys.methods encoding: "*" is the wildcard, anything else is a CSV
// allowlist. An empty string means no capabilities, not all of them.
func ParseCapabilities(v string) CapabilitySet {
	if strings.TrimSpace(v) == "*" {
		return AllCapabilities()
	}
	s := CapabilitySet{set: map[Capability]bool{}}
	for _, name := range strings.Split(v, ",") {
		if name = strings.TrimSpace(name); name != "" {
			s.set[Capability(name)] = true
		}
	}
	return s
}

// String encodes the set for storage. Round-trips through ParseCapabilities.
func (s CapabilitySet) String() string {
	if s.all {
		return "*"
	}
	names := make([]string, 0, len(s.set))
	for c := range s.set {
		names = append(names, string(c))
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// Allows reports whether the device exposes c.
func (s CapabilitySet) Allows(c Capability) bool { return s.all || s.set[c] }

// All reports whether this is the wildcard set.
func (s CapabilitySet) All() bool { return s.all }

// List returns the granted capabilities in Capabilities order. The wildcard
// expands to every known capability, so callers rendering a UI see the concrete
// set rather than having to special-case "*".
func (s CapabilitySet) List() []Capability {
	out := make([]Capability, 0, len(Capabilities))
	for _, c := range Capabilities {
		if s.Allows(c) {
			out = append(out, c)
		}
	}
	return out
}

// IntersectCapabilities returns the set allowed by BOTH inputs. Used to combine
// a device's own declared ceiling with the account-side grant: either may narrow
// the result, neither may widen it, and a wildcard on one side simply defers to
// the other. Intersection is the only combining rule that preserves "the device
// has the final say" — a union would let the server re-enable something the
// device refused.
func IntersectCapabilities(a, b CapabilitySet) CapabilitySet {
	if a.all {
		return b
	}
	if b.all {
		return a
	}
	out := CapabilitySet{set: make(map[Capability]bool, len(a.set))}
	for c := range a.set {
		if b.set[c] {
			out.set[c] = true
		}
	}
	return out
}

// KnownCapability reports whether c is one this build understands. Used to
// reject typos at the API boundary rather than storing a capability that
// silently never matches.
func KnownCapability(c Capability) bool {
	for _, known := range Capabilities {
		if known == c {
			return true
		}
	}
	return false
}
