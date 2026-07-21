// Package x11 is the Linux desktop backend: screen capture (XGB GetImage) and
// input injection (XTEST FakeInput) over a pure-Go X11 connection — no cgo, no
// libX11 dev headers, no xdotool. It is the analogue of the macOS client's
// ScreenCapture + InputInjection, in the same root-window pixel space so a
// capture and a click share one coordinate system.
package x11

import (
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// XTEST fake-event types (X protocol event codes).
const (
	evKeyPress    byte = 2
	evKeyRelease  byte = 3
	evButtonPress byte = 4
	evButtonRel   byte = 5
	evMotion      byte = 6
)

// keyPos is a physical keycode plus whether it needs Shift held to produce the
// mapped keysym (i.e. the keysym sits in the shifted column of the layout).
type keyPos struct {
	code  xproto.Keycode
	shift bool
}

// Conn is a live X11 connection with the XTEST extension initialized and the
// keyboard layout loaded. Not safe for concurrent use; the dispatcher serializes
// command execution so only one input sequence runs at a time.
type Conn struct {
	c      *xgb.Conn
	root   xproto.Window
	width  int
	height int

	minKeycode xproto.Keycode
	ksPerKc    int
	sym2code   map[xproto.Keysym]keyPos
	shiftCode  xproto.Keycode // keycode producing Shift_L, 0 if none
	haveShift  bool
	spareKC    xproto.Keycode // an all-NoSymbol keycode for Unicode typing, 0 if none
}

// Open dials the X server named by $DISPLAY, initializes XTEST, and loads the
// keyboard layout. The primary screen's root window and geometry define the
// capture/click space.
func Open() (*Conn, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("x11 connect (is DISPLAY set?): %w", err)
	}
	if err := xtest.Init(c); err != nil {
		c.Close()
		return nil, fmt.Errorf("x11 XTEST extension: %w", err)
	}
	setup := xproto.Setup(c)
	screen := setup.DefaultScreen(c)
	conn := &Conn{
		c:          c,
		root:       screen.Root,
		width:      int(screen.WidthInPixels),
		height:     int(screen.HeightInPixels),
		minKeycode: setup.MinKeycode,
	}
	conn.loadKeymap(setup)
	return conn, nil
}

// Close tears down the X connection.
func (c *Conn) Close() {
	if c.c != nil {
		c.c.Close()
	}
}

// Size returns the primary screen dimensions in pixels.
func (c *Conn) Size() (int, int) { return c.width, c.height }

// PointerPos returns the current pointer position in root-window pixels. Used to
// confirm that injected motion actually reached the server.
func (c *Conn) PointerPos() (int, int, error) {
	reply, err := xproto.QueryPointer(c.c, c.root).Reply()
	if err != nil {
		return 0, 0, err
	}
	return int(reply.RootX), int(reply.RootY), nil
}

// loadKeymap builds the keysym → keycode reverse index from the live layout and
// picks a spare keycode for Unicode typing. Failures leave the maps empty; input
// verbs then degrade (recognized keys still work via any populated entries).
func (c *Conn) loadKeymap(setup *xproto.SetupInfo) {
	count := int(setup.MaxKeycode) - int(setup.MinKeycode) + 1
	if count <= 0 {
		return
	}
	reply, err := xproto.GetKeyboardMapping(c.c, setup.MinKeycode, byte(count)).Reply()
	if err != nil || reply.KeysymsPerKeycode == 0 {
		return
	}
	c.ksPerKc = int(reply.KeysymsPerKeycode)
	c.sym2code = make(map[xproto.Keysym]keyPos)
	for i := 0; i < count; i++ {
		kc := xproto.Keycode(int(setup.MinKeycode) + i)
		base := i * c.ksPerKc
		// Column 0 is unshifted, column 1 is shifted. First writer wins so the
		// lowest keycode for a symbol is preferred and the unshifted form is
		// preferred over the shifted one.
		if base < len(reply.Keysyms) {
			if ks := reply.Keysyms[base]; ks != 0 {
				if _, ok := c.sym2code[ks]; !ok {
					c.sym2code[ks] = keyPos{kc, false}
				}
			}
		}
		if base+1 < len(reply.Keysyms) {
			if ks := reply.Keysyms[base+1]; ks != 0 {
				if _, ok := c.sym2code[ks]; !ok {
					c.sym2code[ks] = keyPos{kc, true}
				}
			}
		}
	}
	if pos, ok := c.sym2code[ksShiftL]; ok {
		c.shiftCode, c.haveShift = pos.code, true
	}
	// Find an all-NoSymbol keycode to borrow for arbitrary Unicode input.
	for i := count - 1; i >= 0; i-- {
		base := i * c.ksPerKc
		allZero := true
		for j := 0; j < c.ksPerKc && base+j < len(reply.Keysyms); j++ {
			if reply.Keysyms[base+j] != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			c.spareKC = xproto.Keycode(int(setup.MinKeycode) + i)
			break
		}
	}
}

// fake posts one XTEST fake event. Fire-and-forget; call sync() to flush a batch
// and let the server apply it in order.
func (c *Conn) fake(evType, detail byte, x, y int) {
	xtest.FakeInput(c.c, evType, detail, 0, c.root, int16(x), int16(y), 0)
}

// sync round-trips a trivial request, flushing buffered fake events and blocking
// until the server has processed them — the XSync idiom.
func (c *Conn) sync() { _, _ = xproto.GetInputFocus(c.c).Reply() }
