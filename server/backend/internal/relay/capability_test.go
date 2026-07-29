package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"abacad/internal/protocol"
)

// gateAllowing builds a Gate permitting exactly the listed capabilities.
func gateAllowing(caps ...protocol.Capability) Gate {
	set := protocol.NewCapabilitySet(caps...)
	return func(_ string, need protocol.Capability) error {
		if set.Allows(need) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrCapabilityDenied, need)
	}
}

func gatedConn(t *testing.T, gate Gate) *DeviceConn {
	t.Helper()
	c := newTestConn("d1")
	c.gate = gate
	return c
}

// A denied command must fail before the socket is touched. These conns have no
// websocket at all, so reaching the write would panic — which is exactly the
// assertion: the gate sits ahead of every side effect.
func TestSendDeniedBeforeAnyWrite(t *testing.T) {
	c := gatedConn(t, gateAllowing(protocol.Capability(protocol.MethodScreenshot)))

	if _, err := c.Send(context.Background(), protocol.MethodClick, nil, 0); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("click should be denied, got %v", err)
	}
	if _, err := c.Send(context.Background(), protocol.MethodPushFile, nil, 0); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("push_file should be denied, got %v", err)
	}
}

// A connection that was never registered with a hub has no gate. It must deny
// everything rather than allow everything — a gate that fails open is not a gate.
func TestUngatedConnDeniesEverything(t *testing.T) {
	c := newTestConn("d1") // no hub, so no gate
	for _, m := range protocol.Methods {
		if _, err := c.Send(context.Background(), m, nil, 0); !errors.Is(err, ErrCapabilityDenied) {
			t.Fatalf("%s on an ungated conn should be denied, got %v", m, err)
		}
	}
	if _, err := c.OpenStream(context.Background(), "127.0.0.1:22", protocol.CapSSH); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("ssh stream on an ungated conn should be denied, got %v", err)
	}
}

// Register injects the hub's gate, which is what makes a connection drivable.
func TestRegisterInjectsGate(t *testing.T) {
	h := NewHub(gateAllowing(protocol.CapTunnel))
	c := newTestConn("d1")
	h.Register(c)

	if _, err := c.OpenStream(context.Background(), "10.0.0.1:5432", protocol.CapSSH); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("ssh should be denied by a tunnel-only gate, got %v", err)
	}
}

// The tunnel lane carries two distinct consumers. They are gated separately, so
// the SSH jump cannot ride a /connect grant or vice versa.
func TestTunnelAndSSHGatedSeparately(t *testing.T) {
	sshOnly := gatedConn(t, gateAllowing(protocol.CapSSH))
	if _, err := sshOnly.OpenStream(context.Background(), "10.0.0.1:5432", protocol.CapTunnel); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("tunnel should be denied by an ssh-only gate, got %v", err)
	}

	tunnelOnly := gatedConn(t, gateAllowing(protocol.CapTunnel))
	if _, err := tunnelOnly.OpenStream(context.Background(), "127.0.0.1:22", protocol.CapSSH); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("ssh should be denied by a tunnel-only gate, got %v", err)
	}
}

// A denial must reach the activity trail. Silent denials let a compromised agent
// map the whole capability matrix without leaving a trace.
func TestDenialIsRecorded(t *testing.T) {
	c := gatedConn(t, gateAllowing())
	var got CommandRecord
	c.onCmd = func(r CommandRecord) { got = r }

	if _, err := c.Send(context.Background(), protocol.MethodScreenshot, nil, 0); err == nil {
		t.Fatal("expected denial")
	}
	if got.Outcome != "denied" {
		t.Fatalf("outcome = %q, want %q", got.Outcome, "denied")
	}
	if got.Method != string(protocol.MethodScreenshot) {
		t.Fatalf("method = %q", got.Method)
	}
}

// composite carries its own action list, so authorizing the outer verb alone
// would let one permitted call perform every denied one.
func TestCompositeStepsAreAuthorizedIndividually(t *testing.T) {
	// composite + click allowed, but NOT screenshot or typing.
	c := gatedConn(t, gateAllowing(
		protocol.Capability(protocol.MethodComposite),
		protocol.Capability(protocol.MethodClick),
		protocol.Capability(protocol.MethodDrag),
	))

	steps := []map[string]any{{"op": "click", "x": 1.0, "y": 2.0}}
	if _, err := c.authorizeComposite(map[string]any{"steps": steps}); err != nil {
		t.Fatalf("click step should be allowed: %v", err)
	}

	for _, denied := range []map[string]any{
		{"op": "screenshot"},
		{"op": "type", "text": "secret"},
		{"op": "key_down", "key": "cmd"},
	} {
		_, err := c.authorizeComposite(map[string]any{"steps": []map[string]any{denied}})
		if !errors.Is(err, ErrCapabilityDenied) {
			t.Errorf("composite step %v should be denied, got %v", denied, err)
		}
	}
}

// An op with no mapping is rejected, not forwarded. Otherwise a new client-side
// op could ship ahead of its server-side mapping and be unguarded in the gap.
func TestCompositeRejectsUnknownOp(t *testing.T) {
	c := gatedConn(t, AllowAllGate)
	for _, bad := range []map[string]any{
		{"op": "exfiltrate"},
		{"op": 42},
		{"x": 1.0}, // no op at all
	} {
		if _, err := c.authorizeComposite(map[string]any{"steps": []map[string]any{bad}}); err == nil {
			t.Errorf("step %v should be rejected", bad)
		}
	}
}

// Send's timeout stops the server waiting; it does not stop the device
// executing. Without a bound, a permitted composite is a device-DoS primitive.
func TestCompositeBounds(t *testing.T) {
	c := gatedConn(t, AllowAllGate)

	many := make([]map[string]any, maxCompositeSteps+1)
	for i := range many {
		many[i] = map[string]any{"op": "wait", "ms": 0.0}
	}
	if _, err := c.authorizeComposite(map[string]any{"steps": many}); err == nil {
		t.Error("step count over the limit should be rejected")
	}

	long := []map[string]any{{"op": "wait", "ms": float64(maxCompositeWaitMS + 1)}}
	if _, err := c.authorizeComposite(map[string]any{"steps": long}); err == nil {
		t.Error("total wait over the limit should be rejected")
	}

	neg := []map[string]any{{"op": "wait", "ms": -1.0}}
	if _, err := c.authorizeComposite(map[string]any{"steps": neg}); err == nil {
		t.Error("negative wait should be rejected")
	}

	if _, err := c.authorizeComposite(map[string]any{"steps": []map[string]any{}}); err == nil {
		t.Error("empty step list should be rejected")
	}
}

// What is validated must be what is sent. Validating a decoded copy while
// forwarding the original bytes is the classic check-one-thing/send-another hole,
// and the step schema is an open object so the raw form is attacker-shaped.
func TestCompositeForwardsTheValidatedSteps(t *testing.T) {
	c := gatedConn(t, AllowAllGate)
	// Duplicate keys: encoding/json takes the last, so a validator reading the
	// raw bytes separately from the device could disagree about which op ran.
	raw := []map[string]any{{"op": "wait", "ms": 5.0}}
	out, err := c.authorizeComposite(map[string]any{"steps": raw, "other": "kept"})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	steps, ok := out["steps"].([]map[string]any)
	if !ok {
		t.Fatalf("steps were not replaced with the decoded copy: %T", out["steps"])
	}
	if len(steps) != 1 || steps[0]["op"] != "wait" {
		t.Fatalf("unexpected steps: %v", steps)
	}
	if out["other"] != "kept" {
		t.Error("other params should be preserved")
	}
}

// Denial messages must say what was turned off, so the agent can report
// something actionable instead of retrying blindly.
func TestDenialNamesTheCapability(t *testing.T) {
	c := gatedConn(t, gateAllowing())
	_, err := c.Send(context.Background(), protocol.MethodExecute, nil, 0)
	if err == nil || !strings.Contains(err.Error(), string(protocol.MethodExecute)) {
		t.Fatalf("error should name the capability, got %v", err)
	}
}
