package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"abacad/internal/protocol"
	"abacad/internal/relay"
)

// commandTimeout bounds how long an MCP tool waits for a device reply. It is
// generous — far longer than the dashboard's fail-fast default (relay.DefaultTimeout)
// — because an agent's screenshot can be a heavy capture on a busy device, and a
// late frame is far more useful to the agent than a spurious "timed out" error it
// then retries (which only piles more work on the device).
const commandTimeout = 60 * time.Second

// DeviceResolver ties an authenticated MCP request to the devices it may reach.
// The handler builds one per request from the bearer token; the dispatcher uses
// it so tools never see another account's devices.
type DeviceResolver interface {
	// Resolve returns the live connection for an optional device_id. Empty
	// deviceID means "pick my default": the sole device, else the most-recently
	// active one that is online. Errors are surfaced to the agent as tool errors.
	Resolve(ctx context.Context, deviceID string) (*relay.DeviceConn, error)
	// List returns a summary of the account's devices for the list_devices tool.
	List(ctx context.Context) ([]DeviceSummary, error)
}

// DeviceSummary is one row of list_devices output.
type DeviceSummary struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	Online   bool   `json:"online"`
	Platform string `json:"platform,omitempty"` // e.g. "android", "macos"; blank if unset
	LastSeen string `json:"last_seen,omitempty"`
}

// deviceIDArg is the optional target-selector present on every action tool.
type deviceIDArg struct {
	DeviceID string `json:"device_id"`
}

const deviceIDSchema = `"device_id":{"type":"string","description":"which device to target (from list_devices); omit to use your only / most-recently-active device"}`

// actionTool is a device-driving tool. call receives the already-resolved
// connection for the target device.
type actionTool struct {
	name        string
	description string
	schema      string // JSON Schema object
	call        func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult
}

// actionTools are the device operations exposed to an agent: the original mobile
// verbs plus the desktop verbs (click/right_click/drag/scroll/press_keys/composite).
// The list is a global superset — a device answers the subset it implements and
// rejects the rest as "unknown method", so no per-platform filtering is needed.
// Each tool gains an optional device_id selector for multi-device accounts.
var actionTools = []actionTool{
	{
		name:        "screenshot",
		description: "Look at the connected device's screen. Returns a JPEG of the current screen and, by default, the accessibility UI tree: the foreground package plus a list of nodes, each with class, text, resource id, a clickable flag, and screen bounds [left, top, right, bottom]. Use the tree to decide what to interact with — tap the center of a node's bounds. Set include_ui_tree=false for canvas/game screens where the tree is empty or noise (you still get the image). The device is woken automatically if its screen was off.",
		schema:      `{"type":"object","properties":{"include_ui_tree":{"type":"boolean","description":"also return the accessibility UI tree (default true)"},` + deviceIDSchema + `},"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				IncludeUITree *bool `json:"include_ui_tree"`
			}
			_ = json.Unmarshal(args, &a)
			includeTree := a.IncludeUITree == nil || *a.IncludeUITree
			raw, err := dc.Send(ctx, protocol.MethodScreenshot, map[string]any{"include_ui_tree": includeTree}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.ScreenshotResult
			if err := json.Unmarshal(raw, &r); err != nil {
				return errorResult("bad screenshot result: " + err.Error())
			}
			out := toolResult{Content: []content{
				imageContent(r.PNGBase64, "image/jpeg"),
				textContent(fmt.Sprintf("screen %dx%d", r.W, r.H)),
			}}
			if r.Tree != nil {
				treeJSON, _ := json.MarshalIndent(r.Tree, "", "  ")
				out.Content = append(out.Content, textContent(string(treeJSON)))
			}
			return out
		},
	},
	{
		name:        "tap",
		description: "Tap the connected device screen at absolute pixel coordinates. Get coordinates from a screenshot's UI tree node bounds — tap the center of the target node.",
		schema:      `{"type":"object","properties":{"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"},` + deviceIDSchema + `},"required":["x","y"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct{ X, Y int }
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			raw, err := dc.Send(ctx, protocol.MethodTap, map[string]any{"x": a.X, "y": a.Y}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GestureResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("tap dispatched=%v at (%d, %d)", r.Dispatched, a.X, a.Y))
		},
	},
	{
		name:        "long_press",
		description: "Press and hold at absolute pixel coordinates for duration_ms (default 600). Use for context menus, drag handles, and other press-and-hold interactions where a plain tap won't do.",
		schema:      `{"type":"object","properties":{"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"},"duration_ms":{"type":"integer","description":"hold duration in ms (default 600)"},` + deviceIDSchema + `},"required":["x","y"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				X, Y       int
				DurationMs *int `json:"duration_ms"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			dur := 600
			if a.DurationMs != nil {
				dur = *a.DurationMs
			}
			raw, err := dc.Send(ctx, protocol.MethodLongPress, map[string]any{"x": a.X, "y": a.Y, "duration_ms": dur}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GestureResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("long_press dispatched=%v at (%d, %d) %dms", r.Dispatched, a.X, a.Y, dur))
		},
	},
	{
		name:        "swipe",
		description: "Swipe/drag on the connected device from (x1,y1) to (x2,y2) over duration_ms (default 300). Use for scrolling and navigation — e.g. to advance a vertical video feed, swipe from a lower point to a higher point (bottom -> top); a shorter duration flings faster. Absolute pixels; get screen size from a screenshot.",
		schema:      `{"type":"object","properties":{"x1":{"type":"integer","description":"start x pixel"},"y1":{"type":"integer","description":"start y pixel"},"x2":{"type":"integer","description":"end x pixel"},"y2":{"type":"integer","description":"end y pixel"},"duration_ms":{"type":"integer","description":"gesture duration in ms (default 300)"},` + deviceIDSchema + `},"required":["x1","y1","x2","y2"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				X1, Y1, X2, Y2 int
				DurationMs     *int `json:"duration_ms"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			dur := 300
			if a.DurationMs != nil {
				dur = *a.DurationMs
			}
			raw, err := dc.Send(ctx, protocol.MethodSwipe, map[string]any{"x1": a.X1, "y1": a.Y1, "x2": a.X2, "y2": a.Y2, "duration_ms": dur}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GestureResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("swipe dispatched=%v (%d,%d)->(%d,%d) %dms", r.Dispatched, a.X1, a.Y1, a.X2, a.Y2, dur))
		},
	},
	{
		name:        "input_text",
		description: "Type text into the currently focused input field on the connected device. Tap the field first to focus it, then call this. Replaces the field's current contents. For submitting/searching, follow with the on-screen action button (e.g. tap the keyboard's Enter/Search key via its node).",
		schema:      `{"type":"object","properties":{"text":{"type":"string","description":"text to place into the focused field"},` + deviceIDSchema + `},"required":["text"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			raw, err := dc.Send(ctx, protocol.MethodInputText, map[string]any{"text": a.Text}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.InputTextResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("input_text set=%v", r.Set))
		},
	},
	globalAction("back", protocol.MethodBack, "Press the Android Back navigation key: go back one step / dismiss the current screen or keyboard."),
	globalAction("home", protocol.MethodHome, "Press the Android Home navigation key: go to the launcher home screen."),
	globalAction("recents", protocol.MethodRecents, "Press the Android Recents (overview) navigation key: open the recent-apps switcher."),

	// --- Desktop tools (macOS today; a mobile device rejects them as unknown). ---
	{
		name:        "click",
		description: "(desktop) Left-click at absolute pixel coordinates, optionally holding modifier keys (for ⇧-click, ⌘-click, etc.). Get coordinates from a screenshot's UI tree node bounds — click the center of the target. Set count=2 for a double-click.",
		schema:      `{"type":"object","properties":{"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"},"modifiers":{"type":"array","items":{"type":"string","enum":["cmd","shift","opt","ctrl"]},"description":"modifier keys held during the click"},"count":{"type":"integer","description":"click count (2 = double-click; default 1)"},` + deviceIDSchema + `},"required":["x","y"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				X, Y      int
				Modifiers []string `json:"modifiers"`
				Count     *int     `json:"count"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			count := 1
			if a.Count != nil {
				count = *a.Count
			}
			raw, err := dc.Send(ctx, protocol.MethodClick, map[string]any{"x": a.X, "y": a.Y, "modifiers": a.Modifiers, "count": count}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GestureResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("click dispatched=%v at (%d, %d)", r.Dispatched, a.X, a.Y))
		},
	},
	{
		name:        "right_click",
		description: "(desktop) Right / secondary click at absolute pixel coordinates to open a context menu.",
		schema:      `{"type":"object","properties":{"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"},` + deviceIDSchema + `},"required":["x","y"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct{ X, Y int }
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			raw, err := dc.Send(ctx, protocol.MethodRightClick, map[string]any{"x": a.X, "y": a.Y}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GestureResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("right_click dispatched=%v at (%d, %d)", r.Dispatched, a.X, a.Y))
		},
	},
	{
		name:        "drag",
		description: "(desktop) Press at (x1,y1), move to (x2,y2), and release — move a window, select a range, or drag-and-drop. duration_ms (default 300) paces the movement; modifiers are held for the duration.",
		schema:      `{"type":"object","properties":{"x1":{"type":"integer"},"y1":{"type":"integer"},"x2":{"type":"integer"},"y2":{"type":"integer"},"duration_ms":{"type":"integer","description":"drag duration in ms (default 300)"},"modifiers":{"type":"array","items":{"type":"string","enum":["cmd","shift","opt","ctrl"]}},` + deviceIDSchema + `},"required":["x1","y1","x2","y2"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				X1, Y1, X2, Y2 int
				DurationMs     *int     `json:"duration_ms"`
				Modifiers      []string `json:"modifiers"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			dur := 300
			if a.DurationMs != nil {
				dur = *a.DurationMs
			}
			raw, err := dc.Send(ctx, protocol.MethodDrag, map[string]any{"x1": a.X1, "y1": a.Y1, "x2": a.X2, "y2": a.Y2, "duration_ms": dur, "modifiers": a.Modifiers}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GestureResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("drag dispatched=%v (%d,%d)->(%d,%d) %dms", r.Dispatched, a.X1, a.Y1, a.X2, a.Y2, dur))
		},
	},
	{
		name:        "scroll",
		description: "(desktop) Scroll at absolute pixel coordinates by a wheel delta. Positive dy scrolls content up (finger-down / page moves up); negative dy scrolls down. dx scrolls horizontally. Units are wheel lines.",
		schema:      `{"type":"object","properties":{"x":{"type":"integer"},"y":{"type":"integer"},"dx":{"type":"integer","description":"horizontal wheel delta (default 0)"},"dy":{"type":"integer","description":"vertical wheel delta"},` + deviceIDSchema + `},"required":["x","y","dy"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				X, Y, Dx, Dy int
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			raw, err := dc.Send(ctx, protocol.MethodScroll, map[string]any{"x": a.X, "y": a.Y, "dx": a.Dx, "dy": a.Dy}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GestureResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("scroll dispatched=%v at (%d, %d) d=(%d, %d)", r.Dispatched, a.X, a.Y, a.Dx, a.Dy))
		},
	},
	{
		name:        "press_keys",
		description: "(desktop) Press a key chord — a set of keys pressed together and released, like a person hitting ⌘-C or Esc. Use key names (\"cmd\",\"shift\",\"opt\",\"ctrl\",\"enter\",\"tab\",\"esc\",\"space\",\"delete\",\"left\",\"right\",\"up\",\"down\") and single characters (\"c\",\"a\"). Order the modifiers first, then the main key, e.g. [\"cmd\",\"c\"]. For typing prose into a field, use input_text instead.",
		schema:      `{"type":"object","properties":{"keys":{"type":"array","items":{"type":"string"},"description":"keys pressed together as a chord, modifiers first"},` + deviceIDSchema + `},"required":["keys"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				Keys []string `json:"keys"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			if len(a.Keys) == 0 {
				return errorResult("press_keys requires a non-empty keys array")
			}
			raw, err := dc.Send(ctx, protocol.MethodPressKeys, map[string]any{"keys": a.Keys}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.KeyResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("press_keys pressed=%v %v", r.Pressed, a.Keys))
		},
	},
	{
		name:        "composite",
		description: "(desktop) Run an ordered sequence of low-level steps in ONE call, executed on-device with real timing — use for precise, multi-step, or timing-sensitive input that the single-shot verbs can't express, and to batch several actions plus a screenshot into one round-trip. Each step is an object with an \"op\": pointer_down/pointer_move/pointer_up {x,y,button?}, key_down/key_up {key}, type {text}, wait {ms}, click {x,y}, or screenshot {}. Any screenshot steps return their frames in order.",
		schema:      `{"type":"object","properties":{"steps":{"type":"array","items":{"type":"object"},"description":"ordered list of step objects, each with an op field"},` + deviceIDSchema + `},"required":["steps"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				Steps []json.RawMessage `json:"steps"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			if len(a.Steps) == 0 {
				return errorResult("composite requires a non-empty steps array")
			}
			raw, err := dc.Send(ctx, protocol.MethodComposite, map[string]any{"steps": a.Steps}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.CompositeResult
			_ = json.Unmarshal(raw, &r)
			out := toolResult{Content: []content{textContent(fmt.Sprintf("composite ran %d steps, %d screenshot(s)", len(a.Steps), len(r.Shots)))}}
			for i, s := range r.Shots {
				out.Content = append(out.Content, imageContent(s.PNGBase64, "image/jpeg"), textContent(fmt.Sprintf("shot %d: %dx%d", i, s.W, s.H)))
			}
			return out
		},
	},
}

// globalAction builds a no-argument nav-key tool (back / home / recents).
func globalAction(name string, method protocol.Method, description string) actionTool {
	return actionTool{
		name:        name,
		description: description,
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `},"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			raw, err := dc.Send(ctx, method, nil, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.GlobalActionResult
			_ = json.Unmarshal(raw, &r)
			return textResult(fmt.Sprintf("%s performed=%v", name, r.Performed))
		},
	}
}

func textResult(s string) toolResult {
	return toolResult{Content: []content{textContent(s)}}
}

// listDevicesTool describes the account's devices so the agent can pick one.
const listDevicesName = "list_devices"
const listDevicesDescription = "List the devices connected to your Abacad account, with their id, name, platform (e.g. android, macos), and whether they are currently online. Use the platform to pick the right verbs — mobile devices take tap/swipe, desktops take click/scroll/press_keys. Pass a device_id to any other tool to target a specific device; omit it to use your only / most-recently-active device."
const listDevicesSchema = `{"type":"object","properties":{},"additionalProperties":false}`

// toolInfos returns the tools/list payload (list_devices first, then the device
// operations, in a stable order).
func toolInfos() []toolInfo {
	infos := []toolInfo{{
		Name:        listDevicesName,
		Description: listDevicesDescription,
		InputSchema: json.RawMessage(listDevicesSchema),
	}}
	for _, t := range actionTools {
		infos = append(infos, toolInfo{
			Name:        t.name,
			Description: t.description,
			InputSchema: json.RawMessage(t.schema),
		})
	}
	return infos
}

var actionByName = func() map[string]actionTool {
	m := make(map[string]actionTool, len(actionTools))
	for _, t := range actionTools {
		m[t.name] = t
	}
	return m
}()
