// Package frames supplies the RFB server with the device's screen.
//
// iOS exposes no video-capable capture without a kernel tunnel and a codec, so
// the simple and honest source is the developer disk image's screenshot
// service, polled as fast as it answers. A phone answers a 1179x2556 screenshot
// in roughly 100-200 ms over USB, which is enough to tap and swipe with.
package frames

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"sync"
	"time"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/instruments"

	"iosvnc/internal/rfbserver"
)

// shotter is the part of the DVT screenshot service this package uses, so the
// polling and give-up logic can be tested without a device.
type shotter interface {
	TakeScreenshot() ([]byte, error)
	Close()
}

// Screenshot is a Source backed by the DVT screenshot service.
type Screenshot struct {
	svc      shotter
	interval time.Duration
	backoff  time.Duration

	mu       sync.Mutex
	cur      rfbserver.Frame
	size     image.Point
	wait     chan struct{}
	seq      uint64
	dead     chan struct{}
	err      error
	failures int
	closed   bool
}

// maxConsecutiveFailures is how many polls in a row may fail before the source
// gives up. One failure is ordinary (a locked screen, a dropped tunnel); a run
// of them means the device is gone and viewers should be told instead of
// hanging on a frame that will never arrive.
const maxConsecutiveFailures = 10

// OpenScreenshot connects to the screenshot service and takes one frame so the
// size is known before any client connects.
//
// It fails when the developer disk image cannot mount, which on a real device
// almost always means the phone is locked.
func OpenScreenshot(device ios.DeviceEntry, fps int) (*Screenshot, error) {
	svc, err := instruments.NewScreenshotService(device)
	if err != nil {
		return nil, fmt.Errorf("screenshot service: %w (is the device unlocked and the developer disk image mounted?)", err)
	}
	return newScreenshot(svc, fps)
}

// newScreenshot primes the source with one frame so its size is known.
func newScreenshot(svc shotter, fps int) (*Screenshot, error) {
	s := &Screenshot{svc: svc, wait: make(chan struct{}), dead: make(chan struct{}), backoff: time.Second}
	if fps > 0 {
		s.interval = time.Second / time.Duration(fps)
	}
	pngBytes, err := svc.TakeScreenshot()
	if err != nil {
		svc.Close()
		return nil, fmt.Errorf("first screenshot: %w (is the device unlocked?)", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		svc.Close()
		return nil, fmt.Errorf("decode first screenshot: %w", err)
	}
	s.size = img.Bounds().Size()
	s.seq = 1
	s.cur = rfbserver.Frame{Image: img, Seq: s.seq}
	return s, nil
}

// Size reports the device's native framebuffer size.
func (s *Screenshot) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size.X, s.size.Y
}

// Run polls until ctx is cancelled.
func (s *Screenshot) Run(ctx context.Context) {
	for {
		start := time.Now()
		pngBytes, err := s.svc.TakeScreenshot()
		if err == nil {
			var img image.Image
			img, err = png.Decode(bytes.NewReader(pngBytes))
			if err == nil {
				s.publish(img)
			}
		}
		if err != nil {
			// A locked device, a pulled cable and a dropped tunnel all surface
			// here. Back off instead of spinning on a dead service.
			if s.noteFailure(err) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.backoff):
			}
			continue
		}
		s.noteSuccess()
		if wait := s.interval - time.Since(start); wait > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}
}

// noteFailure records a failed poll and reports whether the source has given up.
func (s *Screenshot) noteFailure(err error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
	s.err = err
	if s.failures < maxConsecutiveFailures || s.closed {
		return false
	}
	s.closed = true
	close(s.dead)
	return true
}

func (s *Screenshot) noteSuccess() {
	s.mu.Lock()
	s.failures = 0
	s.err = nil
	s.mu.Unlock()
}

// Err reports why the source stopped, if it did.
func (s *Screenshot) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Screenshot) publish(img image.Image) {
	s.mu.Lock()
	s.seq++
	s.cur = rfbserver.Frame{Image: img, Seq: s.seq}
	close(s.wait)
	s.wait = make(chan struct{})
	s.mu.Unlock()
}

// Wait blocks until a frame newer than after is published.
func (s *Screenshot) Wait(ctx context.Context, after uint64) (rfbserver.Frame, error) {
	for {
		s.mu.Lock()
		if s.cur.Image != nil && s.cur.Seq > after {
			f := s.cur
			s.mu.Unlock()
			return f, nil
		}
		ch := s.wait
		dead := s.dead
		err := s.err
		closed := s.closed
		s.mu.Unlock()

		if closed {
			if err != nil {
				return rfbserver.Frame{}, fmt.Errorf("capture stopped: %w", err)
			}
			return rfbserver.Frame{}, errors.New("capture stopped")
		}

		select {
		case <-ch:
		case <-dead:
		case <-ctx.Done():
			return rfbserver.Frame{}, ctx.Err()
		}
	}
}

// Close releases the screenshot service.
func (s *Screenshot) Close() {
	s.svc.Close()
}
