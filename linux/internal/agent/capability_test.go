package agent

import (
	"errors"
	"strings"
	"testing"

	"abacad-linux/internal/capability"
)

// restore puts the package-level capability state back after a test mutates it.
func restore(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { capability.Set([]string{capability.Wildcard}) })
}

// The point of the device-side check: a command the local config does not expose
// is refused here, regardless of the server having decided to send it.
func TestDisabledMethodRejected(t *testing.T) {
	restore(t)
	capability.Set([]string{capability.Screenshot})

	if err := checkCapability("screenshot", nil); err != nil {
		t.Fatalf("screenshot should be allowed: %v", err)
	}
	for _, m := range []string{"click", "input_text", "push_file", "execute"} {
		err := checkCapability(m, nil)
		if !errors.Is(err, capability.ErrDisabled) {
			t.Errorf("%s should be refused, got %v", m, err)
		}
	}
}

// An unconfigured device behaves exactly as it did before this feature existed.
// Anything else would take working installs offline on upgrade.
func TestWildcardAllowsEverything(t *testing.T) {
	restore(t)
	capability.Set([]string{capability.Wildcard})
	for _, m := range capability.All {
		if err := checkCapability(m, nil); err != nil {
			t.Errorf("%s should be allowed under the wildcard: %v", m, err)
		}
	}
}

// composite can perform other verbs, so authorizing only the outer method would
// let one permitted call do everything the ceiling forbids.
func TestCompositeStepsChecked(t *testing.T) {
	restore(t)
	capability.Set([]string{capability.Composite, capability.Click, capability.Drag})

	ok := map[string]any{"steps": []any{
		map[string]any{"op": "click", "x": 1.0, "y": 2.0},
		map[string]any{"op": "wait", "ms": 10.0},
	}}
	if err := checkCapability("composite", ok); err != nil {
		t.Fatalf("click+wait composite should be allowed: %v", err)
	}

	for _, bad := range []map[string]any{
		{"op": "screenshot"},
		{"op": "type", "text": "secret"},
		{"op": "key_down", "key": "ctrl"},
	} {
		params := map[string]any{"steps": []any{bad}}
		if err := checkCapability("composite", params); !errors.Is(err, capability.ErrDisabled) {
			t.Errorf("composite step %v should be refused, got %v", bad, err)
		}
	}
}

// An op with no mapping is rejected rather than waved through, so a newer server
// cannot slip an unmapped action past a ceiling this build predates.
func TestCompositeUnknownOpRejected(t *testing.T) {
	restore(t)
	capability.Set([]string{capability.Wildcard})
	params := map[string]any{"steps": []any{map[string]any{"op": "exfiltrate"}}}
	if err := checkCapability("composite", params); err == nil {
		t.Fatal("unknown composite op should be rejected even under the wildcard")
	}
}

// Revoking a capability must not strand the session it was revoking. vnc and
// screen_recording multiplex start/stop, so the stop stays allowed.
func TestStopIsNeverGated(t *testing.T) {
	restore(t)
	capability.Set(nil) // expose nothing

	for _, m := range []string{"vnc", "screen_recording"} {
		if err := checkCapability(m, map[string]any{"action": "stop"}); err != nil {
			t.Errorf("%s stop must be allowed even with nothing exposed: %v", m, err)
		}
		if err := checkCapability(m, map[string]any{"action": "start"}); !errors.Is(err, capability.ErrDisabled) {
			t.Errorf("%s start should be refused, got %v", m, err)
		}
	}
}

// The device sees only host:port, so it infers: its own sshd is the jump's fixed
// target, and a tunnel can reach that port anyway.
func TestTunnelTargetGating(t *testing.T) {
	restore(t)

	capability.Set([]string{capability.SSH})
	if err := checkTunnelTarget("127.0.0.1:22"); err != nil {
		t.Errorf("ssh-only should permit the jump's own target: %v", err)
	}
	if err := checkTunnelTarget("10.0.0.5:5432"); !errors.Is(err, capability.ErrDisabled) {
		t.Errorf("ssh-only must not permit an arbitrary tunnel, got %v", err)
	}

	capability.Set([]string{capability.Tunnel})
	if err := checkTunnelTarget("10.0.0.5:5432"); err != nil {
		t.Errorf("tunnel should permit an arbitrary target: %v", err)
	}
	// Tunnel reaches port 22 on its own; pretending otherwise would be a fiction.
	if err := checkTunnelTarget("127.0.0.1:22"); err != nil {
		t.Errorf("tunnel reaches loopback:22 in reality, so it must here: %v", err)
	}

	capability.Set(nil)
	for _, target := range []string{"127.0.0.1:22", "10.0.0.5:5432"} {
		if err := checkTunnelTarget(target); err == nil {
			t.Errorf("%s should be refused when nothing is exposed", target)
		}
	}
}

// The refusal has to say what was turned off and where to change it, or the
// person reading the agent's transcript cannot act on it.
func TestRefusalIsActionable(t *testing.T) {
	restore(t)
	capability.Set(nil)
	err := checkCapability("screenshot", nil)
	if err == nil {
		t.Fatal("expected refusal")
	}
	if !strings.Contains(err.Error(), "screenshot") {
		t.Errorf("refusal should name the capability: %v", err)
	}
}
