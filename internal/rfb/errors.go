package rfb

import (
	"errors"
	"strings"
)

// TooManySecurityFailuresHint is shown to a human when a server refuses a
// connection because this hub's source IP is temporarily blacklisted. TigerVNC
// reports this as an RFB 3.3 security failure reason string ("Too many security
// failures") after too many connections closed before authenticating.
const TooManySecurityFailuresHint = "Too many security failures: the server temporarily blocked this hub's IP; retry later"

// SecurityError is a failure reported by the remote RFB server's security
// negotiation, including the server's own reason string when it sent one.
type SecurityError struct {
	Reason string
}

func (e *SecurityError) Error() string {
	if e.Reason == "" {
		return "authentication failed"
	}
	return e.Reason
}

// IsTooManySecurityFailures reports whether a server rejected the connection
// because the source IP is temporarily blacklisted. Both the classic TigerVNC
// wording and the reason string this package writes downstream are recognized.
func IsTooManySecurityFailures(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "too many security failures") ||
		strings.Contains(s, "temporarily blocked") ||
		strings.Contains(s, "blacklist")
}

// AuthError marks a failure that happened during security negotiation or
// authentication. Only these failures count toward a server's blacklist;
// dial/banner/timeout failures do not.
type AuthError struct {
	Err error
}

func (e *AuthError) Error() string { return e.Err.Error() }
func (e *AuthError) Unwrap() error { return e.Err }

// IsAuthFailure reports whether err was an authentication failure.
func IsAuthFailure(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}

// DownstreamAuthError marks a failure in the client-to-hub handshake. The
// device was never contacted, so it must not change the device's online state.
type DownstreamAuthError struct {
	Err error
}

func (e *DownstreamAuthError) Error() string { return e.Err.Error() }
func (e *DownstreamAuthError) Unwrap() error { return e.Err }

// IsDownstreamAuthFailure reports whether err came from the downstream side.
func IsDownstreamAuthFailure(err error) bool {
	var de *DownstreamAuthError
	return errors.As(err, &de)
}
