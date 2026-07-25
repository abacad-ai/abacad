package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// testMissBudget is the tolerated run of missed pongs in these tests. It matches
// the production pongMissBudget so the "rides out a miss" case exercises the
// real policy, just on millisecond timings.
const testMissBudget = pongMissBudget

// serveConn stands up a one-shot httptest server that accepts a single device
// socket, wires a DeviceConn with the given probe timings, hands it back on the
// channel, and runs its ReadPump (which starts the ping loop). It returns the
// channel and the ws:// URL to dial.
func serveConn(t *testing.T, interval, timeout time.Duration) (<-chan *DeviceConn, string) {
	t.Helper()
	connCh := make(chan *DeviceConn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		dc := NewDeviceConn("d1", c)
		dc.pingInterval = interval
		dc.pongTimeout = timeout
		dc.pongMissBudget = testMissBudget
		connCh <- dc
		dc.ReadPump(context.Background())
	}))
	t.Cleanup(srv.Close)
	return connCh, "ws" + strings.TrimPrefix(srv.URL, "http")
}

// A device that stops answering (frozen in Doze, radio gone) leaves the server's
// Read blocked with no error. Once the whole miss budget is spent the ping loop
// must close the socket, so the hub drops it → offline, not a lingering "asleep".
func TestPingLoopClosesUnresponsiveDevice(t *testing.T) {
	ctx := context.Background()
	connCh, url := serveConn(t, 20*time.Millisecond, 20*time.Millisecond)

	cli, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close(websocket.StatusNormalClosure, "")
	// Deliberately never Read from cli: coder/websocket only auto-pongs during a
	// Read, so a silent client cannot answer the server's ping.

	dc := <-connCh
	select {
	case <-dc.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("ping loop did not close an unresponsive device")
	}
	if r := dc.CloseReason(); !strings.Contains(r, "no pong") {
		t.Fatalf("close reason = %q, want it to mention 'no pong'", r)
	}
}

// A device that keeps answering pongs (asleep but alive, thanks to the wakelock)
// must stay connected across many ping cycles — that's what makes "asleep"
// distinct from "offline".
func TestPingLoopKeepsResponsiveDevice(t *testing.T) {
	ctx := context.Background()
	connCh, url := serveConn(t, 20*time.Millisecond, 40*time.Millisecond)

	cli, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close(websocket.StatusNormalClosure, "")
	// Drain reads so the client auto-pongs to each server ping.
	go func() {
		for {
			if _, _, err := cli.Read(ctx); err != nil {
				return
			}
		}
	}()

	dc := <-connCh
	select {
	case <-dc.closed:
		t.Fatalf("responsive device was closed: %q", dc.CloseReason())
	case <-time.After(300 * time.Millisecond): // ~15 ping cycles at 20ms
	}
}

// The case that made idle devices churn: a device that goes quiet for one probe
// (a Doze window, a Mac nap, a handover) and then answers again must KEEP its
// socket. Dropping on the first miss meant every overnight idle produced a
// disconnect + reconnect pair in the activity log.
func TestPingLoopRidesOutAMissedPong(t *testing.T) {
	ctx := context.Background()
	connCh, url := serveConn(t, 50*time.Millisecond, 20*time.Millisecond)

	cli, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close(websocket.StatusNormalClosure, "")

	// Stay silent past the first probe (it goes unanswered — no Read, no auto-pong),
	// then start reading so every later ping is answered.
	go func() {
		time.Sleep(90 * time.Millisecond) // misses the probe at 50ms, catches the one at 100ms
		for {
			if _, _, err := cli.Read(ctx); err != nil {
				return
			}
		}
	}()

	dc := <-connCh
	select {
	case <-dc.closed:
		t.Fatalf("device was closed after a single missed pong: %q", dc.CloseReason())
	case <-time.After(400 * time.Millisecond): // several probes past the recovery
	}
	// Without this the test could pass having never missed a probe (a fast machine
	// starting the reader before the first deadline), proving nothing.
	if n := dc.missedPongs.Load(); n == 0 {
		t.Fatal("no probe was ever missed — the test didn't exercise the miss budget")
	}
}
