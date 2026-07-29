package relay

import (
	"context"
	"errors"
	"testing"

	"abacad/internal/protocol"
)

// The actor rides the caller's context because the command record is emitted
// inside Send — and, for a relayed command, consumed on a goroutine servicing the
// *device's* socket, where the calling agent's request is long gone. If this link
// breaks, every command row silently loses its attribution while still looking
// complete, so pin it.
func TestSendCarriesActorToCommandRecord(t *testing.T) {
	c := newTestConn("d1")
	c.gate = AllowAllGate // a conn is only drivable once a hub has given it a gate
	close(c.closed)       // makes Send return ErrDeviceGone without any I/O

	var got CommandRecord
	c.SetCommandObserver(func(rec CommandRecord) { got = rec })

	want := Actor{
		Kind: "apikey", ID: "apikey_aaa", Label: "laptop agent",
		IP: "203.0.113.9", UserAgent: "abacad-cli/1.0",
	}
	ctx := WithActor(WithSource(context.Background(), "agent"), want)
	if _, err := c.Send(ctx, protocol.MethodScreenshot, nil, 0); err != ErrDeviceGone {
		t.Fatalf("Send err = %v, want ErrDeviceGone", err)
	}

	if got.Actor != want {
		t.Errorf("actor = %+v, want %+v", got.Actor, want)
	}
	if got.Source != "agent" {
		t.Errorf("source = %q, want agent", got.Source)
	}
	// Attribution must not come at the cost of the existing fields.
	if got.DeviceID != "d1" || got.Outcome != "device_gone" {
		t.Errorf("record lost its basics: %+v", got)
	}
}

// A command issued without a stamped actor still records — with a blank actor
// rather than a wrong one. Un-stamped paths degrade to today's behaviour instead
// of inheriting whatever the last caller happened to set.
func TestSendWithoutActorRecordsBlank(t *testing.T) {
	c := newTestConn("d1")
	c.gate = AllowAllGate
	close(c.closed)

	var got CommandRecord
	c.SetCommandObserver(func(rec CommandRecord) { got = rec })
	if _, err := c.Send(context.Background(), protocol.MethodScreenshot, nil, 0); err != ErrDeviceGone {
		t.Fatalf("Send err = %v, want ErrDeviceGone", err)
	}
	if (got.Actor != Actor{}) {
		t.Errorf("actor = %+v, want zero", got.Actor)
	}
	if got.Source != "agent" {
		t.Errorf("source = %q, want the agent default", got.Source)
	}
}

// A command refused by the capability gate is still recorded, and must still name
// who tried: a denial is the single most audit-worthy row the trail carries, and
// an unattributed one answers none of the questions you would be asking.
func TestDeniedCommandStillCarriesActor(t *testing.T) {
	c := newTestConn("d1")
	c.gate = func(string, protocol.Capability) error { return errors.New("nope") }

	var got CommandRecord
	c.SetCommandObserver(func(rec CommandRecord) { got = rec })

	want := Actor{Kind: "apikey", ID: "apikey_aaa", Label: "laptop agent", IP: "203.0.113.9"}
	ctx := WithActor(context.Background(), want)
	if _, err := c.Send(ctx, protocol.MethodScreenshot, nil, 0); err == nil {
		t.Fatal("Send should fail when the gate denies")
	}
	if got.Actor != want {
		t.Errorf("denied command lost its actor: %+v", got.Actor)
	}
	if got.Outcome == "" {
		t.Error("denied command recorded no outcome")
	}
}
