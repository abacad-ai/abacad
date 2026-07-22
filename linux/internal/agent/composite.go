package agent

import (
	"fmt"
	"time"

	"abacad-linux/internal/x11"
)

// runComposite executes an ordered composite step list on-device with real
// timing — the low-level primitive the named verbs are sugar over. Steps carry
// an "op":
//
//	pointer_down {x,y,button?}   pointer_move {x,y}   pointer_up {button?}
//	key_down {key}   key_up {key}   type {text}
//	click {x,y,button?,count?,modifiers?}   wait {ms}   screenshot {}
//
// Any screenshot steps return their frames in order under {"shots": [...]}.
// Single-pointer only (no multi-touch on X11 XTEST), matching macOS.
func runComposite(x *x11.Conn, steps []map[string]any) (map[string]any, error) {
	shots := []any{}
	buttonDown := false
	var lastX, lastY int

	for _, step := range steps {
		switch op := paramStr(step, "op", ""); op {
		case "pointer_down":
			lastX, lastY = paramInt(step, "x", 0), paramInt(step, "y", 0)
			x.PointerDown(lastX, lastY, x11.ButtonForName(paramStr(step, "button", "left")))
			buttonDown = true
		case "pointer_move":
			lastX, lastY = paramInt(step, "x", 0), paramInt(step, "y", 0)
			x.PointerMove(lastX, lastY)
		case "pointer_up":
			x.PointerUp(lastX, lastY, x11.ButtonForName(paramStr(step, "button", "left")))
			buttonDown = false
		case "key_down":
			x.KeyDownName(paramStr(step, "key", ""))
		case "key_up":
			x.KeyUpName(paramStr(step, "key", ""))
		case "type":
			x.TypeText(paramStr(step, "text", ""))
		case "click":
			// Composite is an explicit, agent-authored step sequence — keep it
			// mechanical (no humanized motion) so waypoints land exactly.
			x.Click(paramInt(step, "x", 0), paramInt(step, "y", 0),
				x11.ButtonForName(paramStr(step, "button", "left")),
				paramInt(step, "count", 1), paramStrs(step, "modifiers"), false)
		case "wait":
			if ms := paramInt(step, "ms", 0); ms > 0 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		case "screenshot":
			shot, err := x.Capture()
			if err != nil {
				return nil, err
			}
			shots = append(shots, map[string]any{"w": shot.W, "h": shot.H, "png_base64": shot.Base64})
		default:
			return nil, fmt.Errorf("composite: unknown op %q", op)
		}
	}
	_ = buttonDown // tracked for symmetry with the macOS runner; not read here
	return map[string]any{"shots": shots}, nil
}
