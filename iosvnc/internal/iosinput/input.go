// Package iosinput turns RFB input events into CoreDevice HID reports.
//
// Touch goes through the digitizer service, which Xcode's own device remote
// control uses. Hardware buttons go through the controls service. Keyboard
// input is not wired: go-ios exposes no keyboard HID report yet, so a session
// can tap, drag and go Home but cannot type.
package iosinput

import (
	"sync"
	"time"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/hid"
)

// digitizer and controls are the two device services this package drives, named
// as interfaces so the mapping can be tested without a phone.
type digitizer interface {
	TouchDown(point hid.NormalizedPoint, edge hid.Edge) error
	TouchMove(point hid.NormalizedPoint) error
	TouchUp(point hid.NormalizedPoint) error
	Close() error
}

type controls interface {
	PressButton(b hid.Button, hold time.Duration) error
	Close() error
}

// Touch is an rfbserver.Input for one device.
type Touch struct {
	dig  digitizer
	ctrl controls

	// outW and outH are the framebuffer dimensions the viewer sees; pointer
	// coordinates arrive in that space and are normalised against it.
	outW, outH int

	mu   sync.Mutex
	down bool
	prev uint8
}

// NewTouch connects the digitizer and controls services.
func NewTouch(device ios.DeviceEntry, outW, outH int) (*Touch, error) {
	dig, err := hid.NewDigitizerSession(device, 0)
	if err != nil {
		return nil, err
	}
	ctrl, err := hid.NewControls(device)
	if err != nil {
		dig.Close()
		return nil, err
	}
	return newTouch(dig, ctrl, outW, outH), nil
}

func newTouch(dig digitizer, ctrl controls, outW, outH int) *Touch {
	return &Touch{dig: dig, ctrl: ctrl, outW: outW, outH: outH}
}

// Pointer maps the RFB button mask onto a touch contact.
//
// Left button is a finger: press, drag, lift. Right button is the Home button,
// which is the closest thing a phone has to a second mouse button.
func (t *Touch) Pointer(mask uint8, x, y int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	point := hid.NormalizedPoint{
		X: clamp01(float64(x) / float64(t.outW)),
		Y: clamp01(float64(y) / float64(t.outH)),
	}

	// A right-click edge fires Home once, on press, not on release.
	if mask&2 != 0 && t.prev&2 == 0 {
		if err := t.ctrl.PressButton(hid.ButtonHome, 60*time.Millisecond); err != nil {
			return err
		}
	}
	t.prev = mask

	finger := mask&1 != 0
	switch {
	case finger && !t.down:
		t.down = true
		return t.dig.TouchDown(point, hid.EdgeNone)
	case finger && t.down:
		return t.dig.TouchMove(point)
	case !finger && t.down:
		t.down = false
		return t.dig.TouchUp(point)
	}
	return nil
}

// Key is deliberately narrow: Escape and Home press the hardware Home button so
// a viewer without a pointer can still get out of an app.
func (t *Touch) Key(down bool, keysym uint32) error {
	if !down {
		return nil
	}
	switch keysym {
	case 0xff1b, 0xff50: // Escape, Home
		return t.ctrl.PressButton(hid.ButtonHome, 60*time.Millisecond)
	}
	return nil
}

// Close lifts any contact and releases both services.
func (t *Touch) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dig != nil {
		_ = t.dig.Close()
	}
	if t.ctrl != nil {
		_ = t.ctrl.Close()
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
