package x11

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestModifierKeysym(t *testing.T) {
	cases := map[string]xproto.Keysym{
		"shift": ksShiftL, "ctrl": ksControlL, "control": ksControlL,
		"alt": ksAltL, "option": ksAltL,
		"cmd": ksSuperL, "meta": ksSuperL, "super": ksSuperL,
	}
	for name, want := range cases {
		ks, ok := modifierKeysym(name)
		if !ok || ks != want {
			t.Errorf("modifierKeysym(%q) = %#x,%v want %#x", name, ks, ok, want)
		}
	}
	if _, ok := modifierKeysym("a"); ok {
		t.Errorf("modifierKeysym(a) should not be a modifier")
	}
}

func TestKeysymForName(t *testing.T) {
	if ks, ok := keysymForName("enter"); !ok || ks != 0xff0d {
		t.Errorf("enter -> %#x,%v", ks, ok)
	}
	if ks, ok := keysymForName("F5"); !ok || ks != 0xffc2 {
		t.Errorf("F5 -> %#x,%v", ks, ok)
	}
	if ks, ok := keysymForName("a"); !ok || ks != 0x61 {
		t.Errorf("a -> %#x,%v", ks, ok)
	}
	if _, ok := keysymForName("notakey"); ok {
		t.Errorf("notakey should not resolve")
	}
}

func TestKeysymForRune(t *testing.T) {
	if got := keysymForRune('A'); got != 0x41 {
		t.Errorf("A -> %#x, want 0x41", got)
	}
	if got := keysymForRune('€'); got != 0x010020ac {
		t.Errorf("€ -> %#x, want 0x010020ac", got)
	}
}

func TestButtonForName(t *testing.T) {
	if buttonForName("right") != 3 {
		t.Errorf("right button != 3")
	}
	if buttonForName("left") != 1 || buttonForName("") != 1 {
		t.Errorf("left/default button != 1")
	}
}
