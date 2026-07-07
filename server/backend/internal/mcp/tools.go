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

// actionTools are the eight device operations, mirroring server/src/mcp.ts. Each
// gains an optional device_id selector for multi-device accounts.
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
const listDevicesDescription = "List the devices connected to your Abacad account, with their id, name, and whether they are currently online. Pass a device_id to any other tool to target a specific device; omit it to use your only / most-recently-active device."
const listDevicesSchema = `{"type":"object","properties":{},"additionalProperties":false}`

// toolInfos returns the tools/list payload (list_devices first, then the eight
// device operations, in a stable order).
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
