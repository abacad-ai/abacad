package agent

import (
	"encoding/json"
	"fmt"
	"net"

	"abacad-linux/internal/capability"
)

// protocolCodeCapabilityDisabled is the Reply.Code returned when this device
// refuses a command because its local configuration does not expose it. Must
// match protocol.ErrCodeCapabilityDisabled on the server.
const protocolCodeCapabilityDisabled = "capability_disabled"

// compositeOps maps each composite step to the capabilities it needs, mirroring
// the server's table. Present here too because composite is the one verb that
// can perform other verbs: authorizing only the outer method would let a
// permitted composite do everything the ceiling forbids.
//
// An op with no entry is REJECTED. Failing closed on an unknown op means a newer
// server (or a hand-crafted frame) cannot slip an unmapped action past a ceiling
// this build does not yet understand.
var compositeOps = map[string][]string{
	"pointer_down": {capability.Click, capability.Drag},
	"pointer_move": {capability.Click, capability.Drag},
	"pointer_up":   {capability.Click, capability.Drag},
	"click":        {capability.Click},
	"tap":          {capability.Tap},
	"long_press":   {capability.LongPress},
	"swipe":        {capability.Swipe},
	"drag":         {capability.Drag},
	"scroll":       {capability.Scroll},
	"key_down":     {capability.PressKeys},
	"key_up":       {capability.PressKeys},
	"type":         {capability.InputText},
	"screenshot":   {capability.Screenshot},
	"wait":         nil,
}

// reportCapabilities tells the server what this device exposes. Sent on connect
// and again on every local change.
//
// Always the full set, never a delta: a delta needs both ends to agree on a
// starting point, and after a reconnect or a dropped frame they do not. Sending
// everything makes the most recent frame the whole truth.
//
// Fire-and-forget, and that is safe — this is an advisory mirror for the
// dashboard and the server's gate, not the enforcement itself. If it never
// arrives, the device still refuses the same commands locally; the only cost is
// that the server does not know why yet.
func (a *Agent) reportCapabilities() {
	frame := map[string]any{
		"type":         "capabilities",
		"capabilities": capability.Report(),
	}
	b, err := json.Marshal(frame)
	if err != nil {
		return
	}
	a.ws.sendText(string(b))
}

// checkCapability authorizes one inbound command against the local ceiling.
func checkCapability(method string, params map[string]any) error {
	// Stopping is never blocked. vnc and screen_recording multiplex start/stop
	// through one method, so refusing the stop would strand the very session the
	// operator just revoked — the screen would stay shared with no way to end it
	// short of quitting. A ceiling prevents things starting, never ceasing.
	if method == "vnc" || method == "screen_recording" {
		if action, _ := params["action"].(string); action == "stop" {
			return nil
		}
	}
	if err := capability.Check(method); err != nil {
		return err
	}
	if method != "composite" {
		return nil
	}
	steps, ok := params["steps"].([]any)
	if !ok {
		return nil // malformed; the dispatcher rejects it on its own terms
	}
	for i, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("composite: step %d is not an object", i)
		}
		op, ok := step["op"].(string)
		if !ok {
			return fmt.Errorf("composite: step %d has no op", i)
		}
		needed, known := compositeOps[op]
		if !known {
			return fmt.Errorf("composite: step %d has unknown op %q", i, op)
		}
		for _, need := range needed {
			if err := capability.Check(need); err != nil {
				return fmt.Errorf("composite: step %d (%s): %w", i, op, err)
			}
		}
	}
	return nil
}

// checkTunnelTarget authorizes a tunnel dial against the local ceiling.
//
// The device sees only a host:port, not which server-side consumer asked, so it
// cannot distinguish the SSH jump from a plain /connect the way the relay can.
// It infers instead: the jump always targets this machine's own sshd, so a
// loopback :22 dial is allowed by EITHER ssh or tunnel, and anything else needs
// tunnel. That mirrors the real relationship — a tunnel can reach port 22 on its
// own, so treating them as independent here would be a fiction.
func checkTunnelTarget(target string) error {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("tunnel target %q is malformed", target)
	}
	if port == "22" && isLoopback(host) {
		if capability.Allows(capability.SSH) || capability.Allows(capability.Tunnel) {
			return nil
		}
		return fmt.Errorf("ssh is %w — turn it back on in the abacad app on this device", capability.ErrDisabled)
	}
	return capability.Check(capability.Tunnel)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
