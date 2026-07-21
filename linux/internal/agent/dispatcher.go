package agent

import (
	"fmt"
	"sync"

	"abacad-linux/internal/x11"
)

// dispatcher routes a parsed {id, method, params} command to a handler and
// produces the result object (or an error). It answers the same superset as the
// macOS client: the mobile verbs (mapped onto desktop input so today's tools
// work unchanged) plus the desktop-native verbs. Anything unrecognized returns
// "unknown method: X" — which is how the server keeps one global tool list
// without per-platform filtering.
//
// Execution is serialized: X11 input is a stateful sequence of fake events, so
// running two commands at once could interleave their events. One in-flight
// command at a time also means the screenshot cache never needs single-flight.
type dispatcher struct {
	x     *x11.Conn
	cache *shotCache
	mu    sync.Mutex
}

func newDispatcher(x *x11.Conn) *dispatcher {
	return &dispatcher{x: x, cache: newShotCache(x, emptyTree)}
}

// execute runs a method and returns its result object, or an error.
func (d *dispatcher) execute(method string, params map[string]any) (map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Any non-screenshot command may change the screen, so invalidate the shot
	// cache before running it — the next screenshot must never serve a frame
	// captured before this action. (Matches the macOS and Android clients.)
	if method != "screenshot" {
		d.cache.invalidate()
	}

	switch method {
	case "screenshot":
		return d.cache.screenshot(paramBool(params, "include_ui_tree", true))

	// Mobile verbs, mapped onto desktop input for cross-platform compatibility.
	case "tap":
		d.x.Click(paramInt(params, "x", 0), paramInt(params, "y", 0), 1, 1, nil)
		return dispatched(), nil
	case "long_press":
		d.x.LongPress(paramInt(params, "x", 0), paramInt(params, "y", 0), paramInt(params, "duration_ms", 600))
		return dispatched(), nil
	case "swipe":
		d.x.Drag(paramInt(params, "x1", 0), paramInt(params, "y1", 0),
			paramInt(params, "x2", 0), paramInt(params, "y2", 0),
			paramInt(params, "duration_ms", 300), nil)
		return dispatched(), nil
	case "input_text":
		// No reliable focused-field API without AT-SPI, so we type the text into
		// the focused element rather than replacing its contents. Click the field
		// to focus it first. (v1 limitation — see README.)
		d.x.TypeText(paramStr(params, "text", ""))
		return map[string]any{"set": true}, nil

	// Desktop-native verbs.
	case "click":
		d.x.Click(paramInt(params, "x", 0), paramInt(params, "y", 0), 1,
			paramInt(params, "count", 1), paramStrs(params, "modifiers"))
		return dispatched(), nil
	case "right_click":
		d.x.RightClick(paramInt(params, "x", 0), paramInt(params, "y", 0))
		return dispatched(), nil
	case "drag":
		d.x.Drag(paramInt(params, "x1", 0), paramInt(params, "y1", 0),
			paramInt(params, "x2", 0), paramInt(params, "y2", 0),
			paramInt(params, "duration_ms", 300), paramStrs(params, "modifiers"))
		return dispatched(), nil
	case "scroll":
		d.x.Scroll(paramInt(params, "x", 0), paramInt(params, "y", 0),
			paramInt(params, "dx", 0), paramInt(params, "dy", 0))
		return dispatched(), nil
	case "press_keys":
		keys := paramStrs(params, "keys")
		if len(keys) == 0 {
			return nil, fmt.Errorf("press_keys requires a non-empty keys array")
		}
		if !d.x.PressChord(keys) {
			return nil, fmt.Errorf("press_keys: no recognized key in %v", keys)
		}
		return map[string]any{"pressed": true}, nil
	case "composite":
		steps := paramObjs(params, "steps")
		if len(steps) == 0 {
			return nil, fmt.Errorf("composite requires a non-empty steps array")
		}
		return runComposite(d.x, steps)

	// Mobile navigation keys have no desktop analogue.
	case "back", "home", "recents":
		return nil, fmt.Errorf("%s has no desktop analogue — use click / press_keys", method)

	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func dispatched() map[string]any { return map[string]any{"dispatched": true} }
