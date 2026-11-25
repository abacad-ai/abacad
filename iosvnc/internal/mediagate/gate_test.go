package mediagate

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/danielpaulus/go-ios/ios/display"
	"github.com/google/uuid"
)

// fakeReceiver hands out whatever is pushed into its channel.
type fakeReceiver struct {
	ch        chan []byte
	closeOnce sync.Once
}

func newFakeReceiver() *fakeReceiver {
	return &fakeReceiver{ch: make(chan []byte, 16)}
}

func (r *fakeReceiver) IP() string { return "127.0.0.1" }
func (r *fakeReceiver) Port() int  { return 5000 }

func (r *fakeReceiver) Read(p []byte) (int, error) {
	msg, ok := <-r.ch
	if !ok {
		return 0, io.EOF
	}
	return copy(p, msg), nil
}

func (r *fakeReceiver) Close() error {
	r.closeOnce.Do(func() { close(r.ch) })
	return nil
}

type fakeStreamer struct {
	startErr  error
	started   int
	stopped   int
	closed    bool
	lastReq   display.VideoStreamRequest
	lastReqMu sync.Mutex
}

func (s *fakeStreamer) StartVideoStream(_ context.Context, req display.VideoStreamRequest) (uuid.UUID, error) {
	s.lastReqMu.Lock()
	s.lastReq = req
	s.lastReqMu.Unlock()
	s.started++
	if s.startErr != nil {
		return uuid.Nil, s.startErr
	}
	return uuid.New(), nil
}

func (s *fakeStreamer) StopMediaStream(context.Context, uuid.UUID) error {
	s.stopped++
	return nil
}

func (s *fakeStreamer) Close() error {
	s.closed = true
	return nil
}

func TestOpenStartsStreamAndDrains(t *testing.T) {
	rc := newFakeReceiver()
	svc := &fakeStreamer{}

	g, err := open(rc, svc, "fd00::1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if svc.started != 1 {
		t.Fatalf("StartVideoStream calls = %d", svc.started)
	}
	if svc.lastReq.ReceiverIP != "127.0.0.1" || svc.lastReq.ReceiverPort != 5000 || svc.lastReq.SenderIP != "fd00::1" {
		t.Fatalf("stream request = %+v", svc.lastReq)
	}

	for i := 0; i < 3; i++ {
		rc.ch <- []byte("rtp")
	}
	deadline := time.Now().Add(2 * time.Second)
	for g.Packets() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := g.Packets(); got != 3 {
		t.Fatalf("packets drained = %d, want 3", got)
	}

	g.Close()
	if svc.stopped != 1 || !svc.closed {
		t.Fatalf("stop=%d closed=%v", svc.stopped, svc.closed)
	}
	select {
	case <-g.Done():
	default:
		t.Fatal("Done not closed after Close")
	}
}

// TestReadErrorEndsGate covers the case that matters on a device: the stream
// dying silently is the difference between input working and being dropped.
func TestReadErrorEndsGate(t *testing.T) {
	rc := newFakeReceiver()
	svc := &fakeStreamer{}
	g, err := open(rc, svc, "fd00::1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	rc.Close() // the socket goes away under the drain loop

	select {
	case <-g.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed after the receiver failed")
	}
	if g.Err() == nil {
		t.Fatal("Err() is nil after the stream ended")
	}
}

func TestStartFailureReleasesReceiver(t *testing.T) {
	rc := newFakeReceiver()
	svc := &fakeStreamer{startErr: errors.New("no media daemon")}

	if _, err := open(rc, svc, "fd00::1", 0); err == nil {
		t.Fatal("open succeeded despite StartVideoStream failing")
	}
	if !svc.closed {
		t.Fatal("streamer was not closed")
	}
	// The receiver must be closed too, otherwise the UDP port leaks.
	if _, err := rc.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("receiver still open: %v", err)
	}
}

func TestAuthSettleIsLongEnough(t *testing.T) {
	// Measured upstream: 0.3 s dropped the first touch on an iPhone 11.
	if AuthSettle < 400*time.Millisecond {
		t.Fatalf("AuthSettle = %v, too short to cover the measured worst case", AuthSettle)
	}
}
