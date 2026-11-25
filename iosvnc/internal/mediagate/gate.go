// Package mediagate opens a throwaway display stream.
//
// iOS publishes the HID digitizer surfaces to a process as
// authenticated: NO until a media stream from that same process is running;
// backboardd then silently drops every touch report ("ignoring digitizer event
// for display main from unsupported service"). go-ios leaves that stream to the
// caller, so a bridge that only takes screenshots can look healthy while every
// tap is discarded.
//
// The stream is only a gate: its RTP payload is drained and thrown away.
package mediagate

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/display"
	"github.com/google/uuid"
)

// AuthSettle is how long to wait after the stream starts before sending the
// first touch. backboardd re-matches the HID surfaces asynchronously and
// nothing reports when it is done. 0.1 s was enough on an iPhone 15 Pro, an
// iPhone 11 needed 0.4 s, and 0.3 s dropped the first touch every time.
const AuthSettle = 500 * time.Millisecond

// startTimeout bounds StartVideoStream, which can hang on a wedged media daemon.
const startTimeout = 20 * time.Second

// Receiver is the UDP socket the device streams RTP to.
type Receiver interface {
	IP() string
	Port() int
	Read(packet []byte) (int, error)
	Close() error
}

// Streamer starts and stops the stream on the device.
type Streamer interface {
	StartVideoStream(ctx context.Context, req display.VideoStreamRequest) (uuid.UUID, error)
	StopMediaStream(ctx context.Context, clientSessionID uuid.UUID) error
	Close() error
}

// Gate holds one stream open for the lifetime of a session.
type Gate struct {
	rc  Receiver
	svc Streamer
	id  uuid.UUID

	stop chan struct{}
	done chan struct{}

	mu      sync.Mutex
	err     error
	packets int64
}

// Open binds a receiver, starts the stream and drains it in the background.
func Open(device ios.DeviceEntry, displayID int) (*Gate, error) {
	rc, err := display.OpenReceiver(device)
	if err != nil {
		return nil, fmt.Errorf("bind RTP receiver: %w", err)
	}
	svc, err := display.New(device)
	if err != nil {
		_ = rc.Close()
		return nil, fmt.Errorf("display service: %w", err)
	}
	return open(rc, svc, device.Address, displayID)
}

// open is Open with the transport injected, so the drain and failure paths can
// be tested without a device.
func open(rc Receiver, svc Streamer, senderIP string, displayID int) (*Gate, error) {
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	id, err := svc.StartVideoStream(ctx, display.VideoStreamRequest{
		ReceiverIP:   rc.IP(),
		ReceiverPort: rc.Port(),
		SenderIP:     senderIP,
		DisplayID:    displayID,
	})
	if err != nil {
		_ = svc.Close()
		_ = rc.Close()
		return nil, fmt.Errorf("start media stream: %w", err)
	}

	g := &Gate{rc: rc, svc: svc, id: id, stop: make(chan struct{}), done: make(chan struct{})}
	go g.drain()
	return g, nil
}

// drain reads and discards packets. Not draining would let the socket buffer
// fill and stall the stream, which would close the auth gate under us.
func (g *Gate) drain() {
	defer close(g.done)
	buf := make([]byte, 64*1024)
	for {
		if _, err := g.rc.Read(buf); err != nil {
			g.mu.Lock()
			if g.err == nil {
				g.err = err
			}
			g.mu.Unlock()
			return
		}
		atomic.AddInt64(&g.packets, 1)
	}
}

// Done is closed when the stream ends, which also ends authenticated input.
func (g *Gate) Done() <-chan struct{} { return g.done }

// Err reports why the stream ended, if it ended by itself.
func (g *Gate) Err() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.err
}

// Packets counts the RTP packets drained so far.
func (g *Gate) Packets() int64 { return atomic.LoadInt64(&g.packets) }

// Close stops the stream and releases the socket. It is safe to call twice.
func (g *Gate) Close() {
	select {
	case <-g.stop:
	default:
		close(g.stop)
	}
	_ = g.rc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = g.svc.StopMediaStream(ctx, g.id)
	_ = g.svc.Close()
	<-g.done
}
