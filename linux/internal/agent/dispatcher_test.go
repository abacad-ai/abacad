package agent

import (
	"strings"
	"testing"
)

// A shell-only device (no X backend, x == nil) must reject every screen/input
// verb with a clear capability error rather than nil-dereferencing the absent
// backend — while non-display concerns are unaffected.
func TestDispatcherShellOnly(t *testing.T) {
	d := newDispatcher(nil) // headless: no display

	for _, m := range []string{
		"screenshot", "tap", "long_press", "swipe", "input_text",
		"click", "right_click", "drag", "scroll", "press_keys", "composite",
	} {
		if _, err := d.execute(m, map[string]any{}); err == nil {
			t.Errorf("%s: expected shell-only error, got nil", m)
		} else if got := err.Error(); !strings.Contains(got, "shell-only") {
			t.Errorf("%s: error %q should mention shell-only", m, got)
		}
	}

	// Non-display verbs keep their existing behavior even with no display.
	if _, err := d.execute("back", nil); err == nil || strings.Contains(err.Error(), "shell-only") {
		t.Errorf("back: want the 'no desktop analogue' error, got %v", err)
	}
	if _, err := d.execute("bogus", nil); err == nil || strings.Contains(err.Error(), "shell-only") {
		t.Errorf("unknown verb: want 'unknown method', got %v", err)
	}
}
