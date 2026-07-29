package relay

import (
	"encoding/json"
	"errors"
	"fmt"

	"abacad/internal/protocol"
)

// ErrCapabilityDenied is returned when a device does not expose the capability a
// command or stream needs. It is deliberately distinguishable from "unknown
// method": the device *could* do this, its owner has said it may not.
var ErrCapabilityDenied = errors.New("capability not exposed by this device")

// Gate reports whether a device currently exposes a capability. It is consulted
// on every Send and every OpenStream — the two points every caller passes
// through, including the ones that never touch an API key's scope (the dashboard
// screenshot, the VNC manager, the blob delivery hook, the SSH jump).
//
// A Gate must read the device's *current* configuration on each call rather than
// close over a snapshot. Capability state is not cached on the connection on
// purpose: a per-connection mirror goes stale the moment the owner revokes
// something, is reset by every reconnect, and — because one DeviceConn is shared
// by every concurrent caller — would let a privileged path populate a permissive
// value that a less privileged one then reads.
type Gate func(deviceID string, c protocol.Capability) error

// AllowAllGate permits everything. Exported so tests and any caller that
// genuinely wants no gating must say so out loud, rather than getting it by
// leaving a field nil.
func AllowAllGate(string, protocol.Capability) error { return nil }

// compositeOps maps each composite step op to the capabilities it needs. The
// vocabulary is fixed and shared by every client (see linux/internal/agent/
// composite.go and macos/Sources/abacad/Composite.swift); an op missing from
// this table is REJECTED, not waved through, so a new client-side op can never
// ship ahead of a server-side mapping.
//
// Raw pointer streams require both drag and click: down/move/up can express
// either, so the strict reading is the only safe one — permitting them under
// "click" alone would make a denied drag reachable by spelling it out longhand.
var compositeOps = map[string][]protocol.Capability{
	"pointer_down": {protocol.Capability(protocol.MethodClick), protocol.Capability(protocol.MethodDrag)},
	"pointer_move": {protocol.Capability(protocol.MethodClick), protocol.Capability(protocol.MethodDrag)},
	"pointer_up":   {protocol.Capability(protocol.MethodClick), protocol.Capability(protocol.MethodDrag)},
	"click":        {protocol.Capability(protocol.MethodClick)},
	"tap":          {protocol.Capability(protocol.MethodTap)},
	"long_press":   {protocol.Capability(protocol.MethodLongPress)},
	"swipe":        {protocol.Capability(protocol.MethodSwipe)},
	"drag":         {protocol.Capability(protocol.MethodDrag)},
	"scroll":       {protocol.Capability(protocol.MethodScroll)},
	"key_down":     {protocol.Capability(protocol.MethodPressKeys)},
	"key_up":       {protocol.Capability(protocol.MethodPressKeys)},
	"type":         {protocol.Capability(protocol.MethodInputText)},
	"screenshot":   {protocol.Capability(protocol.MethodScreenshot)},
	"wait":         nil, // no capability, but still counted against the bounds below
}

const (
	// maxCompositeSteps bounds one call's step list.
	maxCompositeSteps = 256
	// maxCompositeWaitMS bounds the total time a composite may hold the device.
	// Send's timeout only stops the server *waiting* — the device keeps
	// executing — so without this a permitted composite of 10,000 wait steps
	// occupies a device for days regardless of what it is allowed to do.
	maxCompositeWaitMS = 30_000
)

// authorizeComposite checks every step of a composite against the gate and
// returns params with the step list replaced by the decoded, validated copy.
//
// Re-serializing from what was validated is the point: validating a decoded copy
// while forwarding the original bytes is the classic check-one-thing/send-another
// hole, and the step schema is an open `{"type":"object"}` so the raw form is
// entirely attacker-shaped.
func (c *DeviceConn) authorizeComposite(params map[string]any) (map[string]any, error) {
	raw, ok := params["steps"]
	if !ok {
		return params, fmt.Errorf("composite: no steps")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return params, fmt.Errorf("composite: %w", err)
	}
	var steps []map[string]any
	if err := json.Unmarshal(encoded, &steps); err != nil {
		return params, fmt.Errorf("composite: steps must be a list of objects: %w", err)
	}
	if len(steps) == 0 {
		return params, fmt.Errorf("composite: no steps")
	}
	if len(steps) > maxCompositeSteps {
		return params, fmt.Errorf("composite: %d steps exceeds the limit of %d", len(steps), maxCompositeSteps)
	}

	totalWait := 0.0
	for i, step := range steps {
		// Look the op up as an explicit string. Every client does the same exact
		// lookup; decoding into a struct instead would match case-insensitively
		// and let "OP" authorize as one thing and execute as another.
		op, ok := step["op"].(string)
		if !ok {
			return params, fmt.Errorf("composite: step %d has no op", i)
		}
		needed, known := compositeOps[op]
		if !known {
			return params, fmt.Errorf("composite: step %d has unknown op %q", i, op)
		}
		for _, need := range needed {
			if err := c.capability(need); err != nil {
				return params, fmt.Errorf("composite: step %d (%s): %w", i, op, err)
			}
		}
		if op == "wait" {
			ms, _ := step["ms"].(float64)
			if ms < 0 {
				return params, fmt.Errorf("composite: step %d has a negative wait", i)
			}
			totalWait += ms
		}
	}
	if totalWait > maxCompositeWaitMS {
		return params, fmt.Errorf("composite: total wait %.0fms exceeds the limit of %dms", totalWait, maxCompositeWaitMS)
	}

	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = v
	}
	out["steps"] = steps
	return out, nil
}

// authorizeMethod checks a command against the device's capabilities.
//
// Stopping is never gated. `vnc` and `screen_recording` are multiplexers whose
// params carry start/stop, and both drive activity that outlives a single call.
// If revoking a capability also blocked its stop command, turning the capability
// off would strand the very session it was meant to end — a live view that can
// no longer be closed. A gate exists to prevent activity starting, never to
// prevent it ceasing.
func (c *DeviceConn) authorizeMethod(method protocol.Method, params map[string]any) error {
	if method == protocol.MethodVNC || method == protocol.MethodScreenRecording {
		if action, _ := params["action"].(string); action == "stop" {
			return nil
		}
	}
	return c.capability(protocol.Capability(method))
}

// CloseStreams closes every open tunnel stream on this connection, leaving the
// connection itself up.
//
// Tunnels are authorized once, at open time, and then move bytes indefinitely
// (see the connect package doc). So revoking the tunnel or SSH capability has no
// effect on a session already running unless something actively ends it — and a
// toggle that leaves live sessions running is a lie the UI tells its user.
// Closing streams rather than kicking the device keeps the device online and its
// command path intact, which is the narrower and more predictable action.
func (c *DeviceConn) CloseStreams() int {
	c.streamsMu.Lock()
	open := make([]*Stream, 0, len(c.streams))
	for _, s := range c.streams {
		open = append(open, s)
	}
	c.streamsMu.Unlock()
	for _, s := range open {
		s.Close()
	}
	return len(open)
}

// capability checks c against the connection's gate. A missing gate denies:
// a connection that was never registered with a hub has no configured policy,
// and a gate that fails open is not a gate.
func (c *DeviceConn) capability(need protocol.Capability) error {
	if c.gate == nil {
		return fmt.Errorf("%w: %s (no capability gate configured)", ErrCapabilityDenied, need)
	}
	return c.gate(c.DeviceID, need)
}
