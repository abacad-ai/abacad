package relay

import (
	"context"
	"testing"

	"abacad/internal/protocol"
)

// The frame observer is what feeds session replay, and its gate is a per-device
// flag rather than the observer's presence: the observer is installed on every
// connection at /device, so if the gate stopped working, every device in the
// fleet would silently start recording its screen. That is the failure this
// pins.
func TestFrameObserverOnlyFiresWhenReplayIsOn(t *testing.T) {
	c := newTestConn("d1")
	c.gate = AllowAllGate
	close(c.closed) // Send returns ErrDeviceGone without any I/O

	fired := 0
	c.SetFrameObserver(func(FrameRecord) { fired++ })

	// Default: recording off, even though an observer is installed.
	if _, err := c.Send(context.Background(), protocol.MethodScreenshot, nil, 0); err != ErrDeviceGone {
		t.Fatalf("Send err = %v, want ErrDeviceGone", err)
	}
	if fired != 0 {
		t.Fatalf("frame observer fired %d time(s) with replay off", fired)
	}

	c.SetReplay(true)
	if _, err := c.Send(context.Background(), protocol.MethodScreenshot, nil, 0); err != ErrDeviceGone {
		t.Fatalf("Send err = %v, want ErrDeviceGone", err)
	}
	if fired != 1 {
		t.Fatalf("frame observer fired %d time(s) with replay on, want 1", fired)
	}

	// And it goes quiet again the moment recording is turned off — "stop
	// recording" has to mean stopped, not "stopped at the next reconnect".
	c.SetReplay(false)
	if _, err := c.Send(context.Background(), protocol.MethodScreenshot, nil, 0); err != ErrDeviceGone {
		t.Fatalf("Send err = %v, want ErrDeviceGone", err)
	}
	if fired != 1 {
		t.Fatalf("frame observer fired again after replay was turned off (%d)", fired)
	}
}

// The frame record carries the command's parameters and the trail metadata
// together. Params is what lets the player draw "the agent tapped HERE" over the
// frame; without it a recorded tap is an unlocated event.
func TestFrameRecordCarriesParamsAndCommandRecord(t *testing.T) {
	c := newTestConn("d1")
	c.gate = AllowAllGate
	c.SetReplay(true)
	close(c.closed)

	var got FrameRecord
	c.SetFrameObserver(func(rec FrameRecord) { got = rec })

	actor := Actor{Kind: "apikey", ID: "apikey_aaa", Label: "laptop agent"}
	ctx := WithActor(WithSource(context.Background(), "agent"), actor)
	params := map[string]any{"x": 540, "y": 1180, "humanize": false}
	if _, err := c.Send(ctx, protocol.MethodTap, params, 0); err != ErrDeviceGone {
		t.Fatalf("Send err = %v, want ErrDeviceGone", err)
	}

	if got.Params["x"] != 540 || got.Params["y"] != 1180 {
		t.Errorf("params = %+v, want the tap coordinates", got.Params)
	}
	if got.Method != "tap" || got.Source != "agent" || got.Outcome != "device_gone" {
		t.Errorf("command record lost its basics: %+v", got.CommandRecord)
	}
	if got.Actor != actor {
		t.Errorf("actor = %+v, want %+v", got.Actor, actor)
	}
	// A failed command has no reply to keep.
	if got.Result != nil {
		t.Errorf("result = %s, want nil on a failed command", got.Result)
	}
}

// Both observers must fire, and the audit trail must never be the one that
// loses. Replay is opt-in and best-effort; the activity row is neither.
func TestCommandObserverStillFiresWithReplayOn(t *testing.T) {
	c := newTestConn("d1")
	c.gate = AllowAllGate
	c.SetReplay(true)
	close(c.closed)

	var cmd, frame int
	c.SetCommandObserver(func(CommandRecord) { cmd++ })
	c.SetFrameObserver(func(FrameRecord) { frame++ })

	if _, err := c.Send(context.Background(), protocol.MethodScreenshot, nil, 0); err != ErrDeviceGone {
		t.Fatalf("Send err = %v, want ErrDeviceGone", err)
	}
	if cmd != 1 || frame != 1 {
		t.Fatalf("cmd=%d frame=%d, want 1 and 1", cmd, frame)
	}
}
