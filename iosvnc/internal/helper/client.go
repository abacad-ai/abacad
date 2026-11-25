// Package helper drives the Python bridge that owns the iOS device session.
//
// The split is deliberate: the Python side is the only thing that talks to
// CoreDevice (video stream, HID, the media-stream auth gate, keep-awake), and
// this side is the RFB server plus the framing. The wire format is in
// bridge/iosbridge.py: length-prefixed JPEG on stdout, JSON lines on stdin.
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

// Client is a running bridge process.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	frames *bufio.Reader

	writeMu sync.Mutex

	mu    sync.Mutex
	cur   rfbserver.Frame
	wait  chan struct{}
	count uint64
	w, h  int
	err   error
}

// Start spawns the bridge and waits for its first frame, so the RFB server knows
// the framebuffer size before any viewer connects.
func Start(ctx context.Context, python, script, udid string, timeout time.Duration) (*Client, error) {
	args := []string{"-u", script}
	if udid != "" {
		args = append(args, udid)
	}
	cmd := exec.CommandContext(ctx, python, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bridge: %w", err)
	}

	c := &Client{cmd: cmd, stdin: stdin, frames: bufio.NewReaderSize(stdout, 1<<20), wait: make(chan struct{})}
	go c.readLoop()

	// The bridge needs the tunnel, a stream and a keyframe before it can emit
	// anything; 45 s covers the userspace tunnel handshake on a cold start.
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		c.mu.Lock()
		ready := c.count > 0
		failure := c.err
		c.mu.Unlock()
		if ready {
			return c, nil
		}
		if failure != nil {
			c.Close()
			return nil, failure
		}
		select {
		case <-c.wait:
		case <-deadline.C:
			c.Close()
			return nil, errors.New("bridge produced no frame in time")
		case <-ctx.Done():
			c.Close()
			return nil, ctx.Err()
		}
	}
}

func (c *Client) readLoop() {
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(c.frames, hdr[:]); err != nil {
			c.fail(err)
			return
		}
		n := binary.BigEndian.Uint32(hdr[:])
		if n == 0 || n > 16<<20 {
			c.fail(fmt.Errorf("bad frame length %d", n))
			return
		}
		jpeg := make([]byte, n)
		if _, err := io.ReadFull(c.frames, jpeg); err != nil {
			c.fail(err)
			return
		}
		img, _, err := image.Decode(bytes.NewReader(jpeg))
		if err != nil {
			c.fail(fmt.Errorf("decode jpeg: %w", err))
			return
		}
		b := img.Bounds()
		rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		// draw.Draw uses the fast paths; a per-pixel img.At loop costs ~3 M
		// interface calls per frame and backs the pipe up until the bridge
		// starts dropping frames.
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
		c.mu.Lock()
		if c.count == 0 {
			c.w, c.h = b.Dx(), b.Dy()
		}
		c.count++
		c.cur = rfbserver.Frame{Image: rgba, Seq: c.count}
		close(c.wait)
		c.wait = make(chan struct{})
		c.mu.Unlock()
	}
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	close(c.wait)
	c.wait = make(chan struct{})
	c.mu.Unlock()
}

// Size is the framebuffer size of the first frame.
func (c *Client) Size() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.w, c.h
}

// Wait blocks until a frame newer than after exists.
func (c *Client) Wait(ctx context.Context, after uint64) (rfbserver.Frame, error) {
	for {
		c.mu.Lock()
		if c.count > after {
			f := c.cur
			c.mu.Unlock()
			return f, nil
		}
		err := c.err
		ch := c.wait
		c.mu.Unlock()
		if err != nil {
			return rfbserver.Frame{}, err
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return rfbserver.Frame{}, ctx.Err()
		}
	}
}

// Input maps viewer coordinates to the device's 0..65535 range and sends them.
type Input struct {
	c                *Client
	outW, outH       int
	nativeW, nativeH int

	mu   sync.Mutex
	down bool
}

// NewInput builds an Input for a server whose output is outW x outH.
func (c *Client) NewInput(outW, outH int) *Input {
	w, h := c.Size()
	return &Input{c: c, outW: outW, outH: outH, nativeW: w, nativeH: h}
}

func (i *Input) send(cmd map[string]any) error {
	b, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	i.c.writeMu.Lock()
	defer i.c.writeMu.Unlock()
	if os.Getenv("IOSVNC_DEBUG_INPUT") != "" {
		fmt.Fprintf(os.Stderr, "input -> %s", b)
	}
	_, err = i.c.stdin.Write(b)
	return err
}

func (i *Input) norm(x, y int) (int, int) {
	if i.outW <= 0 || i.outH <= 0 {
		return 0, 0
	}
	nx := x * 65535 / max(1, i.outW)
	ny := y * 65535 / max(1, i.outH)
	return clamp(nx), clamp(ny)
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

// Pointer reports the current button mask at output coordinates. The helper
// treats "down" and "move" identically (both are a contact at a position), so a
// drag is just a stream of them; only the press/release edges need state.
func (i *Input) Pointer(mask uint8, x, y int) error {
	nx, ny := i.norm(x, y)
	pressed := mask&1 != 0

	i.mu.Lock()
	was := i.down
	i.down = pressed
	i.mu.Unlock()

	op := "move"
	switch {
	case pressed && !was:
		op = "down"
	case !pressed && was:
		op = "up"
	case !pressed:
		return nil // hover: nothing to tell the device
	}
	return i.send(map[string]any{"op": op, "x": nx, "y": ny})
}

// Key is not wired to the device yet; the RFB server tolerates a no-op.
func (i *Input) Key(bool, uint32) error { return nil }

// Close terminates the bridge.
func (c *Client) Close() error {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}
