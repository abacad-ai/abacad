package frames

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"
	"time"
)

// stubShotter is a screenshot service that can be told to fail.
type stubShotter struct {
	mu      sync.Mutex
	fail    bool
	calls   int
	closed  bool
	pngData []byte
}

func newStubShotter(t *testing.T) *stubShotter {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(1, 1, color.RGBA{10, 20, 30, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &stubShotter{pngData: buf.Bytes()}
}

func (s *stubShotter) TakeScreenshot() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.fail {
		return nil, context.DeadlineExceeded
	}
	return s.pngData, nil
}

func (s *stubShotter) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

func (s *stubShotter) setFail(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = v
}

func (s *stubShotter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestNewScreenshotLearnsSize(t *testing.T) {
	svc := newStubShotter(t)
	src, err := newScreenshot(svc, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if w, h := src.Size(); w != 8 || h != 6 {
		t.Fatalf("size = %dx%d, want 8x6", w, h)
	}
	frame, err := src.Wait(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Seq != 1 || frame.Image == nil {
		t.Fatalf("first frame = %+v", frame)
	}
}

// TestSourceGivesUpAfterConsecutiveFailures covers the behaviour a viewer
// depends on: a device that locks up must end the session, not freeze it.
func TestSourceGivesUpAfterConsecutiveFailures(t *testing.T) {
	svc := newStubShotter(t)
	src, err := newScreenshot(svc, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	src.backoff = time.Millisecond
	svc.setFail(true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		src.Run(ctx)
		close(done)
	}()

	// Asking for a frame newer than the one we have must eventually fail.
	_, err = src.Wait(context.Background(), 1)
	if err == nil {
		t.Fatal("Wait returned no error after the source died")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after giving up")
	}
	if svc.count() < maxConsecutiveFailures {
		t.Fatalf("gave up after %d calls, want at least %d", svc.count(), maxConsecutiveFailures)
	}
	if src.Err() == nil {
		t.Fatal("Err() is nil after the source died")
	}
}

// TestSourceRecoversFromOneFailure proves a single bad poll does not kill it.
func TestSourceRecoversFromOneFailure(t *testing.T) {
	svc := newStubShotter(t)
	src, err := newScreenshot(svc, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	src.backoff = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.setFail(true)
	go src.Run(ctx)

	// Fail once, then let it work again: a new frame must arrive.
	time.Sleep(2 * time.Millisecond)
	svc.setFail(false)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	frame, err := src.Wait(waitCtx, 1)
	if err != nil {
		t.Fatalf("Wait after recovery: %v", err)
	}
	if frame.Seq < 2 {
		t.Fatalf("frame seq = %d, want >= 2", frame.Seq)
	}
}
