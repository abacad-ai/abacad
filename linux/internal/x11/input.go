package x11

import (
	"time"

	"github.com/jezek/xgb/xproto"
)

// Input injection via XTEST FakeInput, mirroring the macOS InputInjection verbs.
// All coordinates are absolute root-window pixels — the same space Capture()
// returns — so a node's bounds map straight to a click point.

// Click posts count clicks of button at (x,y), optionally with modifiers held.
func (c *Conn) Click(x, y int, button byte, count int, modifiers []string) {
	if button == 0 {
		button = 1
	}
	if count < 1 {
		count = 1
	}
	held := c.pressModifiers(modifiers)
	c.fake(evMotion, 0, x, y)
	for i := 0; i < count; i++ {
		c.fake(evButtonPress, button, x, y)
		c.fake(evButtonRel, button, x, y)
	}
	c.releaseModifiers(held)
	c.sync()
}

// RightClick opens a context menu at (x,y).
func (c *Conn) RightClick(x, y int) { c.Click(x, y, 3, 1, nil) }

// LongPress presses the left button at (x,y), holds for holdMs, and releases.
func (c *Conn) LongPress(x, y, holdMs int) {
	c.fake(evMotion, 0, x, y)
	c.fake(evButtonPress, 1, x, y)
	c.sync()
	if holdMs > 0 {
		time.Sleep(time.Duration(holdMs) * time.Millisecond)
	}
	c.fake(evButtonRel, 1, x, y)
	c.sync()
}

// Drag presses at (x1,y1), interpolates to (x2,y2) over durationMs, and releases
// — matching the macOS drag stepping (≤60 steps, ~8ms each).
func (c *Conn) Drag(x1, y1, x2, y2, durationMs int, modifiers []string) {
	held := c.pressModifiers(modifiers)
	c.fake(evMotion, 0, x1, y1)
	c.fake(evButtonPress, 1, x1, y1)
	c.sync()
	steps := durationMs / 8
	if steps < 1 {
		steps = 1
	}
	if steps > 60 {
		steps = 60
	}
	perStep := time.Duration(durationMs/steps) * time.Millisecond
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x1 + int(float64(x2-x1)*t)
		y := y1 + int(float64(y2-y1)*t)
		c.fake(evMotion, 0, x, y)
		if perStep > 0 {
			c.sync()
			time.Sleep(perStep)
		}
	}
	c.fake(evButtonRel, 1, x2, y2)
	c.releaseModifiers(held)
	c.sync()
}

// Scroll emits wheel-button clicks at (x,y). X wheels are discrete buttons:
// 4=up, 5=down, 6=left, 7=right. dy/dx are treated as notch counts (capped) so a
// positive dy scrolls up, matching the macOS convention.
func (c *Conn) Scroll(x, y, dx, dy int) {
	c.fake(evMotion, 0, x, y)
	c.wheel(x, y, dy, 4, 5)
	c.wheel(x, y, dx, 7, 6)
	c.sync()
}

func (c *Conn) wheel(x, y, delta int, pos, neg byte) {
	btn := pos
	if delta < 0 {
		delta, btn = -delta, neg
	}
	if delta > 100 {
		delta = 100
	}
	for i := 0; i < delta; i++ {
		c.fake(evButtonPress, btn, x, y)
		c.fake(evButtonRel, btn, x, y)
	}
}

// PressChord presses a chord: modifiers held while the main key(s) go down then
// up. Returns false if no main (non-modifier) key was recognized.
func (c *Conn) PressChord(keys []string) bool {
	var mods []xproto.Keycode
	var mains []keyPos
	for _, k := range keys {
		if ks, ok := modifierKeysym(k); ok {
			if pos, ok := c.sym2code[ks]; ok {
				mods = append(mods, pos.code)
			}
			continue
		}
		if ks, ok := keysymForName(k); ok {
			if pos, ok := c.sym2code[ks]; ok {
				mains = append(mains, pos)
			}
		}
	}
	if len(mains) == 0 {
		return false
	}
	needShift := false
	for _, m := range mains {
		if m.shift {
			needShift = true
		}
	}
	for _, m := range mods {
		c.fake(evKeyPress, byte(m), 0, 0)
	}
	if needShift && c.haveShift {
		c.fake(evKeyPress, byte(c.shiftCode), 0, 0)
	}
	for _, m := range mains {
		c.fake(evKeyPress, byte(m.code), 0, 0)
	}
	for i := len(mains) - 1; i >= 0; i-- {
		c.fake(evKeyRelease, byte(mains[i].code), 0, 0)
	}
	if needShift && c.haveShift {
		c.fake(evKeyRelease, byte(c.shiftCode), 0, 0)
	}
	for i := len(mods) - 1; i >= 0; i-- {
		c.fake(evKeyRelease, byte(mods[i]), 0, 0)
	}
	c.sync()
	return true
}

// TypeText types a Unicode string as keystrokes. Characters present in the live
// layout are pressed directly (with Shift when needed); anything else is typed
// via a temporarily remapped spare keycode — the xdotool trick.
func (c *Conn) TypeText(text string) {
	for _, r := range text {
		ks := keysymForRune(r)
		if pos, ok := c.sym2code[ks]; ok {
			if pos.shift && c.haveShift {
				c.fake(evKeyPress, byte(c.shiftCode), 0, 0)
			}
			c.fake(evKeyPress, byte(pos.code), 0, 0)
			c.fake(evKeyRelease, byte(pos.code), 0, 0)
			if pos.shift && c.haveShift {
				c.fake(evKeyRelease, byte(c.shiftCode), 0, 0)
			}
		} else {
			c.typeViaRemap(ks)
		}
		c.sync()
	}
}

// typeViaRemap borrows the spare keycode: remap it to the target keysym, tap it,
// then restore it to NoSymbol so the layout is left as we found it.
func (c *Conn) typeViaRemap(ks xproto.Keysym) {
	if c.spareKC == 0 || c.ksPerKc == 0 {
		return
	}
	syms := make([]xproto.Keysym, c.ksPerKc)
	for i := range syms {
		syms[i] = ks // same symbol in every column so Shift state is irrelevant
	}
	xproto.ChangeKeyboardMapping(c.c, 1, c.spareKC, byte(c.ksPerKc), syms)
	c.sync()
	c.fake(evKeyPress, byte(c.spareKC), 0, 0)
	c.fake(evKeyRelease, byte(c.spareKC), 0, 0)
	c.sync()
	xproto.ChangeKeyboardMapping(c.c, 1, c.spareKC, byte(c.ksPerKc), make([]xproto.Keysym, c.ksPerKc))
	c.sync()
}

// --- low-level primitives for composite ---

// PointerDown / PointerMove / PointerUp drive a single pointer explicitly.
func (c *Conn) PointerDown(x, y int, button byte) {
	if button == 0 {
		button = 1
	}
	c.fake(evMotion, 0, x, y)
	c.fake(evButtonPress, button, x, y)
	c.sync()
}

func (c *Conn) PointerMove(x, y int) {
	c.fake(evMotion, 0, x, y)
	c.sync()
}

func (c *Conn) PointerUp(x, y int, button byte) {
	if button == 0 {
		button = 1
	}
	c.fake(evButtonRel, button, x, y)
	c.sync()
}

// KeyDownName / KeyUpName press or release a single named key (modifier or main).
func (c *Conn) KeyDownName(name string) { c.keyByName(name, true) }
func (c *Conn) KeyUpName(name string)   { c.keyByName(name, false) }

// ButtonForName exposes the pointer-name → X button mapping to callers building
// composite steps.
func ButtonForName(name string) byte { return buttonForName(name) }

func (c *Conn) keyByName(name string, down bool) {
	ev := evKeyRelease
	if down {
		ev = evKeyPress
	}
	if ks, ok := modifierKeysym(name); ok {
		if pos, ok := c.sym2code[ks]; ok {
			c.fake(ev, byte(pos.code), 0, 0)
		}
		c.sync()
		return
	}
	if ks, ok := keysymForName(name); ok {
		if pos, ok := c.sym2code[ks]; ok {
			c.fake(ev, byte(pos.code), 0, 0)
		}
	}
	c.sync()
}

// pressModifiers presses each recognized modifier and returns the keycodes held,
// for releaseModifiers to undo in reverse.
func (c *Conn) pressModifiers(names []string) []xproto.Keycode {
	var held []xproto.Keycode
	for _, n := range names {
		if ks, ok := modifierKeysym(n); ok {
			if pos, ok := c.sym2code[ks]; ok {
				c.fake(evKeyPress, byte(pos.code), 0, 0)
				held = append(held, pos.code)
			}
		}
	}
	return held
}

func (c *Conn) releaseModifiers(held []xproto.Keycode) {
	for i := len(held) - 1; i >= 0; i-- {
		c.fake(evKeyRelease, byte(held[i]), 0, 0)
	}
}
