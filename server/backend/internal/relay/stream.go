package relay

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/coder/websocket"

	"abacad/internal/protocol"
)

// maxStreamFrame bounds one server->device DATA payload, so a large client write
// is chopped into modest WebSocket messages rather than one giant frame. The
// device chooses its own chunking for the reverse direction.
const maxStreamFrame = 32 << 10

// streamBufferFrames is how many inbound payloads a stream buffers before it
// applies backpressure (blocking the connection's ReadPump). Enough to smooth
// bursts without hoarding memory per idle stream.
const streamBufferFrames = 256

// Stream is one tunneled TCP connection multiplexed over the device WebSocket.
// It implements io.ReadWriteCloser: Write sends bytes to the device end, Read
// yields bytes the device sent back, Close tears it down both ways.
//
// The relay never interprets these bytes — it is a blind mover — so an SSH or
// TLS session tunneled through stays end-to-end encrypted and the server never
// needs a key.
type Stream struct {
	id   uint64
	conn *DeviceConn

	in       chan []byte // inbound device payloads; ReadPump feeds this
	leftover []byte      // tail of a payload a Read didn't fully consume

	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error // readable only after closed is closed
}

// Read yields bytes the device sent back. It returns the stream's close error
// (io.EOF for a clean close) once the device end is done and the buffer drained.
func (s *Stream) Read(p []byte) (int, error) {
	if len(s.leftover) > 0 {
		n := copy(p, s.leftover)
		s.leftover = s.leftover[n:]
		return n, nil
	}
	select {
	case b := <-s.in:
		return s.deliverRead(p, b), nil
	case <-s.closed:
		// Deliver anything buffered before the close, then report the error.
		select {
		case b := <-s.in:
			return s.deliverRead(p, b), nil
		default:
			return 0, s.closeErr
		}
	}
}

func (s *Stream) deliverRead(p, b []byte) int {
	n := copy(p, b)
	if n < len(b) {
		s.leftover = b[n:]
	}
	return n
}

// Write sends bytes to the device end, chunked to maxStreamFrame per frame.
func (s *Stream) Write(p []byte) (int, error) {
	select {
	case <-s.closed:
		return 0, s.closeErr
	default:
	}
	total := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > maxStreamFrame {
			chunk = p[:maxStreamFrame]
		}
		frame := protocol.EncodeStreamFrame(protocol.StreamData, s.id, chunk)
		if err := s.conn.writeFrame(context.Background(), websocket.MessageBinary, frame); err != nil {
			s.finish(ErrDeviceGone, false)
			return total, ErrDeviceGone
		}
		total += len(chunk)
		p = p[len(chunk):]
	}
	return total, nil
}

// Close tears the stream down and notifies the device with a CLOSE frame.
func (s *Stream) Close() error { return s.finish(io.EOF, true) }

// finish closes the stream exactly once. sendClose sends a CLOSE frame to the
// device (true for a local Close; false when the close was triggered BY the
// device or a dropped connection, where a frame would be pointless or racy).
func (s *Stream) finish(cause error, sendClose bool) error {
	s.closeOnce.Do(func() {
		s.closeErr = cause
		if sendClose {
			frame := protocol.EncodeStreamFrame(protocol.StreamClose, s.id, nil)
			_ = s.conn.writeFrame(context.Background(), websocket.MessageBinary, frame)
		}
		s.conn.removeStream(s.id)
		close(s.closed)
	})
	return nil
}

// deliver hands an inbound payload to a waiting Read, blocking (backpressure) if
// the buffer is full, and giving up if the stream closes meanwhile.
func (s *Stream) deliver(b []byte) {
	select {
	case s.in <- b:
	case <-s.closed:
	}
}

// closeCause maps a StreamClose payload to a Read error: empty is a clean EOF, a
// reason string becomes an error.
func closeCause(reason []byte) error {
	if len(reason) == 0 {
		return io.EOF
	}
	return errors.New(string(reason))
}
