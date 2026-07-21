package x11

import (
	"strings"

	"github.com/jezek/xgb/xproto"
)

// Maps friendly key names (as an agent would send them) to X11 keysyms, the
// Linux analogue of the macOS KeyMap. Resolution to a physical keycode happens
// against the live keyboard mapping loaded in conn.go, so this file stays a pure
// name → keysym table. US-layout assumptions match the other clients.

// X keysyms for the modifier keys we accept. Values are the standard
// keysymdef.h constants (Shift_L, Control_L, Alt_L, Super_L).
const (
	ksShiftL   xproto.Keysym = 0xffe1
	ksControlL xproto.Keysym = 0xffe3
	ksAltL     xproto.Keysym = 0xffe9
	ksSuperL   xproto.Keysym = 0xffeb
)

// modifierKeysym returns the keysym for a modifier name, or (0,false) if the
// name is not a modifier. cmd/command/meta/super all fold to Super, matching the
// macOS mapping of ⌘ onto the platform's primary meta key.
func modifierKeysym(name string) (xproto.Keysym, bool) {
	switch strings.ToLower(name) {
	case "shift":
		return ksShiftL, true
	case "ctrl", "control":
		return ksControlL, true
	case "alt", "opt", "option":
		return ksAltL, true
	case "cmd", "command", "meta", "super", "win":
		return ksSuperL, true
	}
	return 0, false
}

// namedKeysym maps named keys to keysymdef.h constants. Covers the same set as
// the macOS KeyMap.named plus a couple of Linux-conventional aliases.
var namedKeysym = map[string]xproto.Keysym{
	"enter": 0xff0d, "return": 0xff0d,
	"tab": 0xff09, "space": 0x0020,
	"backspace": 0xff08, "delete": 0xffff, "forwarddelete": 0xffff,
	"esc": 0xff1b, "escape": 0xff1b,
	"left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54,
	"home": 0xff50, "end": 0xff57, "pageup": 0xff55, "pagedown": 0xff56,
	"insert": 0xff63,
	"f1":     0xffbe, "f2": 0xffbf, "f3": 0xffc0, "f4": 0xffc1,
	"f5": 0xffc2, "f6": 0xffc3, "f7": 0xffc4, "f8": 0xffc5,
	"f9": 0xffc6, "f10": 0xffc7, "f11": 0xffc8, "f12": 0xffc9,
}

// keysymForName resolves a non-modifier key name to a keysym. A named key wins;
// otherwise a single character maps to its keysym (Latin-1 keysyms equal the
// codepoint; other runes use the Unicode keysym range).
func keysymForName(name string) (xproto.Keysym, bool) {
	if ks, ok := namedKeysym[strings.ToLower(name)]; ok {
		return ks, true
	}
	r := []rune(name)
	if len(r) == 1 {
		return keysymForRune(r[0]), true
	}
	return 0, false
}

// keysymForRune maps a Unicode rune to an X keysym. ASCII and Latin-1 keysyms
// are the codepoint itself; everything else uses the 0x01000000 Unicode range.
func keysymForRune(r rune) xproto.Keysym {
	if r > 0 && r < 0x100 {
		return xproto.Keysym(r)
	}
	return xproto.Keysym(0x01000000 | uint32(r))
}

// buttonForName maps a pointer button name to an X button number (1=left,
// 3=right). Unknown names fall back to left, matching the macOS default.
func buttonForName(name string) byte {
	if strings.EqualFold(name, "right") {
		return 3
	}
	return 1
}
