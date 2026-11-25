// Package helper supervises the Python bridge that owns the iOS device session.
//
// The split is deliberate: the Python side is the only thing that talks to
// CoreDevice (video stream, HID, the media-stream auth gate, keep-awake), and
// this side is the RFB server plus the framing. The wire format is in
// bridge/iosbridge.py: length-prefixed JPEG on stdout, JSON lines on stdin.
//
// The bridge is a child process, and children die: the tunnel drops, the device
// is unplugged, a decoder thread wedges. A viewer cannot tell a frozen JPEG
// from a still screen, so a supervisor is not a nicety here -- if this side ever
// keeps serving the last frame after the bridge is gone, the viewer shows a
// live-looking picture of a phone nothing can touch. When the session is down,
// Wait returns an error, which ends every RFB connection, and Ready reports
// false, which refuses new ones.
package helper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"iosvnc/internal/rfbserver"
)

// Config describes how to launch and supervise the bridge process.
type Config struct {
	// Python is the interpreter, Script the bridge to run, UDID the device to
	// pin (empty: the first paired one).
	Python string
	Script string
	UDID   string
	// StartTimeout bounds the wait for the first frame, both at startup and on
	// every restart. A cold userspace tunnel handshake takes a few seconds; a
	// stale one can take a minute before it gives up.
	StartTimeout time.Duration
	// StallTimeout kills a bridge that has produced neither a frame nor a line
	// of log output for this long. The bridge logs a stats line every two
	// seconds, so silence means the process is wedged. A quiet screen is not
	// silence: a still screen sends no frames, but the stats line keeps coming.
	StallTimeout time.Duration
	// Logf receives supervisor events. Nil discards.
	Logf func(format string, args ...any)
}

// child is one bridge process.
type child struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	frames chan rfbserver.Frame
	done   chan error

	mu   sync.Mutex
	last time.Time
}

func (c *child) touch() {
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
}

func (c *child) lastActivity() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

func (c *child) kill() {
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}

// push keeps only the newest frame: the viewer wants the freshest picture, not
// every picture, so an undelivered frame is replaced rather than queued.
func (c *child) push(f rfbserver.Frame) {
	select {
	case c.frames <- f:
		return
	default:
	}
	select {
	case <-c.frames:
	default:
	}
	select {
	case c.frames <- f:
	default:
	}
}

func (c *child) send(cmd map[string]any) error {
	b, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if os.Getenv("IOSVNC_DEBUG_INPUT") != "" {
		fmt.Fprintf(os.Stderr, "input -> %s", b)
	}
	_, err = c.stdin.Write(b)
	return err
}

// Supervisor owns the device session: one bridge at a time, restarted with
// backoff whenever it ends.
type Supervisor struct {
	cfg Config

	mu      sync.Mutex
	cur     *child // nil while the session is down
	frame   rfbserver.Frame
	seq     uint64
	ready   bool
	lastErr error
	w, h    int
	wait    chan struct{} // closed whenever frame/ready changes

	outW, outH int

	downMu sync.Mutex
	down   bool
}

// NewSupervisor returns a supervisor that has not started a bridge yet.
func NewSupervisor(cfg Config) *Supervisor {
	if cfg.StartTimeout <= 0 {
		cfg.StartTimeout = 75 * time.Second
	}
	if cfg.StallTimeout <= 0 {
		cfg.StallTimeout = 15 * time.Second
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Supervisor{cfg: cfg, wait: make(chan struct{})}
}

// Start launches the first bridge and waits for its first frame, so the RFB
// server knows the framebuffer size before any viewer connects. It fails rather
// than serve a device that is not there yet.
func (s *Supervisor) Start(ctx context.Context) error {
	c, err := s.spawn(ctx)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			c.kill()
		}
	}()

	timer := time.NewTimer(s.cfg.StartTimeout)
	defer timer.Stop()
	for {
		select {
		case f := <-c.frames:
			s.install(c)
			s.publish(c, f)
			ok = true
			return nil
		case err := <-c.done:
			if err == nil || errors.Is(err, io.EOF) {
				err = errors.New("bridge exited before the first frame")
			}
			return fmt.Errorf("bridge: %w", err)
		case <-timer.C:
			return fmt.Errorf("bridge produced no frame in %v", s.cfg.StartTimeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Run supervises until ctx is cancelled: it pumps frames out of the current
// bridge and replaces it when it dies.
func (s *Supervisor) Run(ctx context.Context) {
	go s.Watchdog(ctx)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			s.shutdown()
			return
		}
		s.mu.Lock()
		c := s.cur
		s.mu.Unlock()
		if c == nil {
			if err := s.restart(ctx); err != nil {
				s.cfg.Logf("bridge restart failed: %v (retrying in %v)", err, backoff)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					s.shutdown()
					return
				}
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
			backoff = time.Second
			continue
		}
		select {
		case f := <-c.frames:
			s.publish(c, f)
		case err := <-c.done:
			s.drop(c, err)
			c.kill()
		case <-ctx.Done():
			s.shutdown()
			return
		}
	}
}

// restart spawns a bridge and waits for its first frame, then installs it.
func (s *Supervisor) restart(ctx context.Context) error {
	c, err := s.spawn(ctx)
	if err != nil {
		return err
	}
	timer := time.NewTimer(s.cfg.StartTimeout)
	defer timer.Stop()
	for {
		select {
		case f := <-c.frames:
			s.install(c)
			s.publish(c, f)
			s.cfg.Logf("device session restored")
			return nil
		case err := <-c.done:
			if err == nil {
				err = errors.New("bridge exited before the first frame")
			}
			return err
		case <-timer.C:
			c.kill()
			return fmt.Errorf("no frame in %v", s.cfg.StartTimeout)
		case <-ctx.Done():
			c.kill()
			return ctx.Err()
		}
	}
}

// spawn starts one bridge process and its readers.
func (s *Supervisor) spawn(ctx context.Context) (*child, error) {
	args := []string{"-u", s.cfg.Script}
	if s.cfg.UDID != "" {
		args = append(args, s.cfg.UDID)
	}
	cmd := exec.CommandContext(ctx, s.cfg.Python, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bridge: %w", err)
	}

	c := &child{
		cmd:    cmd,
		stdin:  stdin,
		frames: make(chan rfbserver.Frame, 1),
		done:   make(chan error, 1),
		last:   time.Now(),
	}
	go c.readLogs(stderr)
	go func() {
		err := c.readFrames(stdout)
		select {
		case c.done <- err:
		default:
		}
		// Reap the process. Every read that matters has completed by now: the
		// frame pipe is at EOF, which is what tells us the child is gone.
		_ = cmd.Wait()
	}()
	return c, nil
}

// readLogs forwards the bridge's stderr verbatim and uses it as a heartbeat.
func (c *child) readLogs(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		c.touch()
		fmt.Fprintln(os.Stderr, sc.Text())
	}
}

// readFrames decodes the length-prefixed JPEG stream.
func (c *child) readFrames(r io.Reader) error {
	fr := bufio.NewReaderSize(r, 1<<20)
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(fr, hdr[:]); err != nil {
			return err
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n == 0 || n > 16<<20 {
			return fmt.Errorf("bad frame length %d", n)
		}
		jpeg := make([]byte, n)
		if _, err := io.ReadFull(fr, jpeg); err != nil {
			return err
		}
		img, _, err := image.Decode(bytes.NewReader(jpeg))
		if err != nil {
			return fmt.Errorf("decode jpeg: %w", err)
		}
		b := img.Bounds()
		rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		// draw.Draw uses the fast paths; a per-pixel img.At loop costs ~3 M
		// interface calls per frame and backs the pipe up until the bridge
		// starts dropping frames.
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
		c.touch()
		c.push(rfbserver.Frame{Image: rgba})
	}
}

func (s *Supervisor) install(c *child) {
	s.mu.Lock()
	s.cur = c
	s.mu.Unlock()
}

func (s *Supervisor) publish(c *child, f rfbserver.Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != c {
		return
	}
	s.seq++
	f.Seq = s.seq
	s.frame = f
	if b := f.Image.Bounds(); b.Dx() != s.w || b.Dy() != s.h {
		s.w, s.h = b.Dx(), b.Dy()
	}
	if !s.ready {
		s.ready = true
		s.lastErr = nil
	}
	close(s.wait)
	s.wait = make(chan struct{})
}

// drop marks the session dead and wakes every waiter so they fail fast.
func (s *Supervisor) drop(c *child, err error) {
	s.mu.Lock()
	if s.cur != c {
		s.mu.Unlock()
		return
	}
	s.cur = nil
	s.ready = false
	s.lastErr = err
	close(s.wait)
	s.wait = make(chan struct{})
	s.mu.Unlock()
	s.cfg.Logf("device session ended: %v", err)
}

func (s *Supervisor) shutdown() {
	s.mu.Lock()
	c := s.cur
	s.cur = nil
	s.ready = false
	s.lastErr = errors.New("shutting down")
	close(s.wait)
	s.wait = make(chan struct{})
	s.mu.Unlock()
	if c != nil {
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		c.kill()
	}
}

// Watchdog kills a bridge that has gone quiet. A dropped tunnel raises inside
// the bridge and is noticed through the frame pipe; this is for the other
// failure: a process that stays alive with a blocked read, holding its stdout
// open so the pipe never reaches EOF.
func (s *Supervisor) Watchdog(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			c := s.cur
			s.mu.Unlock()
			if c == nil {
				continue
			}
			if idle := time.Since(c.lastActivity()); idle > s.cfg.StallTimeout {
				err := fmt.Errorf("bridge silent for %v", idle.Round(time.Second))
				s.drop(c, err)
				c.kill()
			}
		}
	}
}

// Ready reports whether a bridge is currently producing frames.
func (s *Supervisor) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur != nil && s.ready
}

// Size is the framebuffer size of the most recent frame.
func (s *Supervisor) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w, s.h
}

// Wait returns a frame newer than after, or an error once the session is down.
func (s *Supervisor) Wait(ctx context.Context, after uint64) (rfbserver.Frame, error) {
	for {
		s.mu.Lock()
		if s.frame.Seq > after {
			f := s.frame
			s.mu.Unlock()
			return f, nil
		}
		if s.cur == nil {
			err := s.lastErr
			s.mu.Unlock()
			if err == nil {
				err = errors.New("device session is down")
			}
			return rfbserver.Frame{}, err
		}
		ch := s.wait
		s.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return rfbserver.Frame{}, ctx.Err()
		}
	}
}

// Close ends the current bridge.
func (s *Supervisor) Close() {
	s.shutdown()
}

// SetOutputSize tells the supervisor the viewer-facing size, which is what
// pointer coordinates are reported in.
func (s *Supervisor) SetOutputSize(w, h int) {
	s.mu.Lock()
	s.outW, s.outH = w, h
	s.mu.Unlock()
}

// Pointer forwards a pointer event to the live bridge. Nothing is buffered: an
// event for a session that has ended is dropped, because replaying a stale
// touch into the next session would tap something nobody asked for.
func (s *Supervisor) Pointer(mask uint8, x, y int) error {
	s.mu.Lock()
	c := s.cur
	outW, outH := s.outW, s.outH
	s.mu.Unlock()
	if c == nil {
		return errors.New("device session is down")
	}

	nx, ny := normalize(x, y, outW, outH)

	s.downMu.Lock()
	was := s.down
	s.down = mask&1 != 0
	s.downMu.Unlock()

	op := "move"
	switch {
	case mask&1 != 0 && !was:
		op = "down"
	case mask&1 == 0 && was:
		op = "up"
	case mask&1 == 0:
		return nil // hover: nothing to tell the device
	}
	return c.send(map[string]any{"op": op, "x": nx, "y": ny})
}

// Key is not wired to the device yet; the RFB server tolerates a no-op.
func (s *Supervisor) Key(bool, uint32) error { return nil }

func normalize(x, y, outW, outH int) (int, int) {
	if outW <= 0 || outH <= 0 {
		return 0, 0
	}
	return clamp(x * 65535 / max(1, outW)), clamp(y * 65535 / max(1, outH))
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return v
}
