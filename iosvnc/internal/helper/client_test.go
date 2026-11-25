package helper

import (
	"context"
	"errors"
	"image"
	"io"
	"testing"
	"time"

	"iosvnc/internal/rfbserver"
)

// nopWriteCloser stands in for a bridge's stdin.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// fakeChild is a child whose process has already been reaped.
func fakeChild() *child {
	return &child{
		stdin:  nopWriteCloser{io.Discard},
		frames: make(chan rfbserver.Frame, 1),
		done:   make(chan error, 1),
		last:   time.Now(),
	}
}

func frame() rfbserver.Frame {
	return rfbserver.Frame{Image: image.NewRGBA(image.Rect(0, 0, 4, 2))}
}

// The whole point of the supervisor: when the device session dies, viewers must
// be told, because a frozen frame and a still screen look identical.
func TestSessionDropFailsWaitingViewer(t *testing.T) {
	s := NewSupervisor(Config{})
	c := fakeChild()
	s.install(c)
	s.publish(c, frame())

	f, err := s.Wait(context.Background(), 0)
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if !s.Ready() {
		t.Fatal("not ready after a frame")
	}

	// A viewer showed that frame and is waiting for a newer one when the
	// tunnel drops.
	got := make(chan error, 1)
	go func() {
		_, err := s.Wait(context.Background(), f.Seq)
		got <- err
	}()
	time.Sleep(20 * time.Millisecond)
	s.drop(c, errors.New("tunnel closed"))

	select {
	case err := <-got:
		if err == nil {
			t.Fatal("viewer stayed connected to a dead session")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("viewer was not woken when the session dropped")
	}
	if s.Ready() {
		t.Fatal("ready after the session dropped")
	}
}

// A viewer that connects while the session is down is refused up front instead
// of being handed a stale framebuffer.
func TestNotReadyBeforeFirstFrame(t *testing.T) {
	s := NewSupervisor(Config{})
	if s.Ready() {
		t.Fatal("ready with no bridge")
	}
	s.install(fakeChild())
	if s.Ready() {
		t.Fatal("ready before the first frame arrived")
	}
	if w, h := s.Size(); w != 0 || h != 0 {
		t.Fatalf("size %dx%d before any frame", w, h)
	}
	if err := s.Pointer(1, 5, 5); err != nil {
		t.Fatalf("pointer with a live bridge: %v", err)
	}
}

// Sequence numbers stay monotonic across restarts, so a viewer's lastSeq from
// the old session still orders correctly against frames from the new one.
func TestSeqSurvivesRestart(t *testing.T) {
	s := NewSupervisor(Config{})
	first := fakeChild()
	s.install(first)
	s.publish(first, frame())
	s.publish(first, frame())
	s.drop(first, errors.New("tunnel closed"))

	second := fakeChild()
	s.install(second)
	s.publish(second, frame())

	f, err := s.Wait(context.Background(), 2)
	if err != nil {
		t.Fatalf("frame after restart: %v", err)
	}
	if f.Seq != 3 {
		t.Fatalf("seq %d after restart, want 3", f.Seq)
	}
	if !s.Ready() {
		t.Fatal("not ready after the restart produced a frame")
	}
}

// Events for a session that already ended must not be replayed into the next
// one, and a drop for a stale child must not clobber the live session.
func TestStaleChildIsIgnored(t *testing.T) {
	s := NewSupervisor(Config{})
	old := fakeChild()
	s.install(old)
	s.drop(old, errors.New("gone"))

	live := fakeChild()
	s.install(live)
	s.publish(live, frame())
	s.publish(live, frame())

	s.drop(old, errors.New("late error from the old bridge"))
	if !s.Ready() {
		t.Fatal("a late drop from a replaced bridge killed the live session")
	}
	s.publish(old, frame())
	if f, err := s.Wait(context.Background(), 1); err != nil || f.Seq != 2 {
		t.Fatalf("stale publish leaked: seq=%d err=%v", f.Seq, err)
	}
}

func TestNormalizeClampsToDeviceRange(t *testing.T) {
	cases := []struct{ x, y, w, h, wantX, wantY int }{
		{0, 0, 592, 1280, 0, 0},
		{591, 1279, 592, 1280, 65424, 65483},
		{-5, 99999, 592, 1280, 0, 65535},
		{5, 5, 0, 0, 0, 0},
	}
	for _, c := range cases {
		gx, gy := normalize(c.x, c.y, c.w, c.h)
		if gx != c.wantX || gy != c.wantY {
			t.Errorf("normalize(%d,%d,%d,%d) = %d,%d want %d,%d", c.x, c.y, c.w, c.h, gx, gy, c.wantX, c.wantY)
		}
	}
}

// Pointer edges are derived from the mask, and only edges are sent: the bridge
// turns every report into a contact at that position.
func TestPointerEdgeDetection(t *testing.T) {
	s := NewSupervisor(Config{})
	c := fakeChild()
	s.install(c)
	s.publish(c, frame())
	s.SetOutputSize(592, 1280)

	if err := s.Pointer(0, 10, 10); err != nil {
		t.Fatalf("hover: %v", err)
	}
	if err := s.Pointer(1, 10, 10); err != nil {
		t.Fatalf("press: %v", err)
	}
	if err := s.Pointer(0, 10, 10); err != nil {
		t.Fatalf("release: %v", err)
	}
}
