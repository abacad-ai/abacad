package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

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
	// AccountID is the account this request is scoped to. The file-transfer tools
	// use it to stage and read blobs on the caller's behalf.
	AccountID() string
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
// connection for the target device. A file-transfer tool sets fileCall instead
// of call: it also needs the caller's account and the blob store to move bytes
// through the /blobs data plane on the agent's behalf. Exactly one of call /
// fileCall is set.
type actionTool struct {
	name        string
	description string
	schema      string // JSON Schema object
	call        func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult
	fileCall    func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage, accountID string, blobs BlobStore) toolResult
}

// actionTools are the device operations exposed to an agent: the original mobile
// verbs plus the desktop verbs (click/right_click/drag/scroll/press_keys/composite).
// The list is a global superset — a device answers the subset it implements and
// rejects the rest as "unknown method", so no per-platform filtering is needed.
// Every tool's schema leads with an optional device_id selector (the first
// property, by convention) for multi-device accounts.
var actionTools = []actionTool{
	{
		name:        "screenshot",
		description: "Look at the connected device's screen. Returns a JPEG of the current screen and, by default, the accessibility UI tree: the foreground package plus a list of nodes, each with class, text, resource id, a clickable flag, and screen bounds [left, top, right, bottom]. Use the tree to decide what to interact with — tap the center of a node's bounds. Set include_ui_tree=false for canvas/game screens where the tree is empty or noise (you still get the image). The device is woken automatically if its screen was off.",
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"include_ui_tree":{"type":"boolean","description":"also return the accessibility UI tree (default true)"}},"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"}},"required":["x","y"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"},"duration_ms":{"type":"integer","description":"hold duration in ms (default 600)"}},"required":["x","y"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"x1":{"type":"integer","description":"start x pixel"},"y1":{"type":"integer","description":"start y pixel"},"x2":{"type":"integer","description":"end x pixel"},"y2":{"type":"integer","description":"end y pixel"},"duration_ms":{"type":"integer","description":"gesture duration in ms (default 300)"}},"required":["x1","y1","x2","y2"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"text":{"type":"string","description":"text to place into the focused field"}},"required":["text"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"},"modifiers":{"type":"array","items":{"type":"string","enum":["cmd","shift","opt","ctrl"]},"description":"modifier keys held during the click"},"count":{"type":"integer","description":"click count (2 = double-click; default 1)"}},"required":["x","y"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"x":{"type":"integer","description":"x pixel coordinate"},"y":{"type":"integer","description":"y pixel coordinate"}},"required":["x","y"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"x1":{"type":"integer"},"y1":{"type":"integer"},"x2":{"type":"integer"},"y2":{"type":"integer"},"duration_ms":{"type":"integer","description":"drag duration in ms (default 300)"},"modifiers":{"type":"array","items":{"type":"string","enum":["cmd","shift","opt","ctrl"]}}},"required":["x1","y1","x2","y2"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"x":{"type":"integer"},"y":{"type":"integer"},"dx":{"type":"integer","description":"horizontal wheel delta (default 0)"},"dy":{"type":"integer","description":"vertical wheel delta"}},"required":["x","y","dy"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"keys":{"type":"array","items":{"type":"string"},"description":"keys pressed together as a chord, modifiers first"}},"required":["keys"],"additionalProperties":false}`,
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
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"steps":{"type":"array","items":{"type":"object"},"description":"ordered list of step objects, each with an op field"}},"required":["steps"],"additionalProperties":false}`,
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

	// --- Browser tool (a browser device runs it in its own page; other
	// platforms reject it as unknown). ---
	{
		name:        "execute",
		description: "(browser) Evaluate JavaScript inside the browser device's page and return the JSON-serialized result. This is the browser's power verb — prefer it over pixel clicks for anything structured. The code runs as the body of an async function, so you can return a value and await promises: e.g. return document.title; return [...document.querySelectorAll('a')].map(a => a.href); return await fetch('/api/x').then(r => r.json()). Use it to read page state, act by selector (document.querySelector('#go').click(); el.value = 'hi'), and build content in place (document.body.innerHTML = ...). It always has full control because it runs in the device page itself. Do NOT navigate away — location.href = '...', or clicking/submitting anything that unloads the page: a top-level navigation unloads the device client and takes the device OFFLINE with no way back until someone reopens it. A thrown error is returned as a tool error.",
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"code":{"type":"string","description":"JavaScript to evaluate; runs as an async function body, so use return <value> to get a result back"}},"required":["code"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			if a.Code == "" {
				return errorResult("execute requires a non-empty code string")
			}
			raw, err := dc.Send(ctx, protocol.MethodExecute, map[string]any{"code": a.Code}, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.ExecuteResult
			_ = json.Unmarshal(raw, &r)
			if len(r.Value) == 0 || string(r.Value) == "null" {
				return textResult("execute ok (no value returned)")
			}
			pretty, e := json.MarshalIndent(json.RawMessage(r.Value), "", "  ")
			if e != nil {
				pretty = r.Value
			}
			return textResult(string(pretty))
		},
	},

	// --- File-transfer tools (any device with a filesystem; a browser/mobile
	// device without the verb rejects them as unknown). The bytes ride the
	// /blobs data plane over HTTP, never the device socket — see docs. ---
	{
		name:        "push_file",
		description: "Write a file onto the connected device's filesystem at an absolute path. Provide the bytes inline as text (for a text file) or content_base64 (for binary); exactly one. The device fetches the bytes over HTTP and writes them, so large files are fine. Returns the bytes written and their sha256. Parent directories must already exist. This is a real filesystem write — the path is whatever you pass, subject to the device user's permissions.",
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"path":{"type":"string","description":"absolute destination path on the device"},"content":{"type":"string","description":"file contents as UTF-8 text (use this for text files)"},"content_base64":{"type":"string","description":"file contents as base64 (use this for binary files); mutually exclusive with content"},"mode":{"type":"integer","description":"unix file mode, e.g. 493 for 0755 (default 420 = 0644)"}},"required":["path"],"additionalProperties":false}`,
		fileCall:    pushFile,
	},
	{
		name:        "pull_file",
		description: "Read a file from the connected device's filesystem at an absolute path. The device uploads the bytes over HTTP; small text files (<= 64 KiB) are returned inline, otherwise you get the blob id, size, and sha256 and can fetch the raw bytes from GET /blobs/{id}. Use this to inspect configs, logs, or any file the device user can read.",
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"path":{"type":"string","description":"absolute source path on the device"},"max_inline_bytes":{"type":"integer","description":"cap on inlined text (default 65536); larger files return a blob id only"}},"required":["path"],"additionalProperties":false}`,
		fileCall:    pullFile,
	},

	// --- Screen recording (file channel). Continuous capture, recorded on-device
	// at full quality and uploaded to /blobs on stop; the agent fetches the finished
	// clip from GET /blobs/{id}. The live (VNC) channel is a later addition. ---
	{
		name:        "screen_recording",
		description: "Record the connected device's screen to a high-quality video file — the moving-picture counterpart of screenshot, for capturing a whole flow (an app test, a demo you'll edit into a promo). Drive it with action: \"start\" begins an on-device recording at full resolution and frame rate — pass file={enabled:true} and optionally fps/max_duration_seconds — then keep issuing your normal verbs (tap/click/…) while it records in the background. \"stop\" finalizes the clip and begins transferring it; because a full-quality clip can be large, the upload runs in the background. \"status\" reports progress — poll it after stop until a download link appears, then fetch the bytes from GET /blobs/{id}. One recording per device at a time; video only (no audio).",
		schema:      `{"type":"object","properties":{` + deviceIDSchema + `,"action":{"type":"string","enum":["start","stop","status"],"description":"start a recording, stop and transfer it, or report status"},"file":{"type":"object","description":"the file channel: record to a high-quality on-device video file, transferred afterward","properties":{"enabled":{"type":"boolean","description":"turn the file channel on (required for start)"},"fps":{"type":"integer","description":"frames per second (default = native/max)"},"format":{"type":"string","description":"container/codec (default \"mp4\", H.264)"},"max_duration_seconds":{"type":"integer","description":"safety cap on recording length"}},"additionalProperties":false}},"required":["action"],"additionalProperties":false}`,
		call: func(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage) toolResult {
			var a struct {
				Action string `json:"action"`
				File   *struct {
					Enabled            *bool   `json:"enabled"`
					FPS                *int    `json:"fps"`
					Format             *string `json:"format"`
					MaxDurationSeconds *int    `json:"max_duration_seconds"`
				} `json:"file"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return errorResult("invalid args: " + err.Error())
			}
			params := map[string]any{"action": a.Action}
			switch a.Action {
			case "start":
				// Only the file channel exists today; live is not yet implemented.
				if a.File == nil || a.File.Enabled == nil || !*a.File.Enabled {
					return errorResult("screen_recording start requires file={enabled:true} (the live channel is not implemented yet)")
				}
				file := map[string]any{}
				if a.File.FPS != nil {
					file["fps"] = *a.File.FPS
				}
				if a.File.Format != nil {
					file["format"] = *a.File.Format
				}
				if a.File.MaxDurationSeconds != nil {
					file["max_duration_seconds"] = *a.File.MaxDurationSeconds
				}
				params["file"] = file
			case "stop", "status":
				// no extra params
			default:
				return errorResult(`screen_recording action must be "start", "stop", or "status"`)
			}
			raw, err := dc.Send(ctx, protocol.MethodScreenRecording, params, commandTimeout)
			if err != nil {
				return errorResult(err.Error())
			}
			var r protocol.ScreenRecordingResult
			if err := json.Unmarshal(raw, &r); err != nil {
				return errorResult("bad screen_recording result: " + err.Error())
			}
			return textResult(formatRecording(a.Action, r))
		},
	},
}

// formatRecording renders a screen_recording reply for the agent, surfacing the
// download reference (GET /blobs/{id}) once the finished clip has uploaded, and
// otherwise nudging the agent to poll status while the transfer runs.
func formatRecording(action string, r protocol.ScreenRecordingResult) string {
	if r.Error != "" {
		return fmt.Sprintf("screen_recording %s: state=%s error=%s", action, r.State, r.Error)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "screen_recording %s: state=%s", action, r.State)
	if r.Width > 0 && r.Height > 0 {
		fmt.Fprintf(&b, " %dx%d", r.Width, r.Height)
	}
	if r.FPS > 0 {
		fmt.Fprintf(&b, " @%dfps", r.FPS)
	}
	if r.DurationMs > 0 {
		fmt.Fprintf(&b, " dur=%.1fs", float64(r.DurationMs)/1000)
	} else if r.ElapsedMs > 0 {
		fmt.Fprintf(&b, " elapsed=%.1fs", float64(r.ElapsedMs)/1000)
	}
	if r.SizeBytes > 0 {
		fmt.Fprintf(&b, " %d bytes", r.SizeBytes)
	}
	if r.TransferState != "" {
		fmt.Fprintf(&b, " transfer=%s", r.TransferState)
	}
	if r.BlobID != "" {
		fmt.Fprintf(&b, "\ndownload: GET /blobs/%s", r.BlobID)
		if r.SHA256 != "" {
			fmt.Fprintf(&b, " (sha256 %s)", r.SHA256)
		}
	} else if action == "stop" || r.TransferState == "uploading" {
		b.WriteString("\n(uploading — call screen_recording action=\"status\" until the download link appears)")
	}
	return b.String()
}

// maxInlineDefault bounds how much of a pulled file is returned inline in the
// tool result, to keep a large pull from flooding the agent's context. Beyond
// it (or for non-text bytes) the agent gets the blob id and fetches the rest.
const maxInlineDefault = 64 << 10

// pushFile stages the agent-provided bytes as a blob, then tells the device to
// download that blob and write it to dest. The device verifies the bytes it
// wrote by size + sha256, which we cross-check against what we staged.
func pushFile(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage, accountID string, blobs BlobStore) toolResult {
	var a struct {
		Path          string  `json:"path"`
		Content       *string `json:"content"`
		ContentBase64 *string `json:"content_base64"`
		Mode          *int    `json:"mode"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult("invalid args: " + err.Error())
	}
	if a.Path == "" {
		return errorResult("push_file requires a path")
	}
	if (a.Content == nil) == (a.ContentBase64 == nil) {
		return errorResult("push_file requires exactly one of content or content_base64")
	}

	var data []byte
	if a.Content != nil {
		data = []byte(*a.Content)
	} else {
		b, err := base64.StdEncoding.DecodeString(*a.ContentBase64)
		if err != nil {
			return errorResult("content_base64 is not valid base64: " + err.Error())
		}
		data = b
	}

	blobID, size, sum, err := blobs.Put(accountID, "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		return errorResult("could not stage file: " + err.Error())
	}

	params := map[string]any{"blob_id": blobID, "dest_path": a.Path}
	if a.Mode != nil {
		params["mode"] = *a.Mode
	}
	raw, err := dc.Send(ctx, protocol.MethodPushFile, params, commandTimeout)
	if err != nil {
		return errorResult(err.Error())
	}
	var r protocol.PushFileResult
	_ = json.Unmarshal(raw, &r)
	if r.SHA256 != "" && r.SHA256 != sum {
		return errorResult(fmt.Sprintf("integrity check failed: staged sha256 %s but device wrote %s", sum, r.SHA256))
	}
	return textResult(fmt.Sprintf("wrote %d bytes to %s (sha256 %s)", size, a.Path, sum))
}

// pullFile asks the device to upload the file at path as a blob, then reads that
// blob back and inlines it if it is small UTF-8 text; otherwise it returns the
// handle (blob id + size + sha256) for the agent to fetch on its own terms.
func pullFile(ctx context.Context, dc *relay.DeviceConn, args json.RawMessage, accountID string, blobs BlobStore) toolResult {
	var a struct {
		Path           string `json:"path"`
		MaxInlineBytes *int64 `json:"max_inline_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult("invalid args: " + err.Error())
	}
	if a.Path == "" {
		return errorResult("pull_file requires a path")
	}
	inlineCap := int64(maxInlineDefault)
	if a.MaxInlineBytes != nil && *a.MaxInlineBytes >= 0 {
		inlineCap = *a.MaxInlineBytes
	}

	raw, err := dc.Send(ctx, protocol.MethodPullFile, map[string]any{"src_path": a.Path}, commandTimeout)
	if err != nil {
		return errorResult(err.Error())
	}
	var r protocol.PullFileResult
	if err := json.Unmarshal(raw, &r); err != nil || r.BlobID == "" {
		return errorResult("device did not return a blob for the file")
	}

	handle := fmt.Sprintf("blob_id %s · %d bytes · sha256 %s", r.BlobID, r.Size, r.SHA256)
	if r.Size > inlineCap {
		return textResult(fmt.Sprintf("%s\n(too large to inline; fetch the bytes from GET /blobs/%s)", handle, r.BlobID))
	}

	rc, _, _, err := blobs.Open(accountID, r.BlobID)
	if err != nil {
		return errorResult("could not read the uploaded blob: " + err.Error())
	}
	defer rc.Close()
	content, err := io.ReadAll(io.LimitReader(rc, inlineCap))
	if err != nil {
		return errorResult("could not read the uploaded blob: " + err.Error())
	}
	if !utf8.Valid(content) {
		return textResult(fmt.Sprintf("%s\n(binary content; fetch the bytes from GET /blobs/%s)", handle, r.BlobID))
	}
	return textResult(fmt.Sprintf("%s\n\n%s", handle, content))
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
const listDevicesDescription = "List the devices connected to your abacad account, with their id, name, platform (e.g. android, macos, browser), and whether they are currently online. Use the platform to pick the right verbs — mobile devices take tap/swipe, desktops take click/scroll/press_keys, and a browser device is best driven with execute (run JS in the page) alongside screenshot/click/scroll. Pass a device_id to any other tool to target a specific device; omit it to use your only / most-recently-active device."
const listDevicesSchema = `{"type":"object","properties":{},"additionalProperties":false}`

// toolInfos returns the tools/list payload (list_devices first, then the device
// operations, in a stable order), filtered to the methods this key's scope
// permits so the agent only sees tools it can actually call. list_devices is
// always present.
func toolInfos(scope Scope) []toolInfo {
	infos := []toolInfo{{
		Name:        listDevicesName,
		Description: listDevicesDescription,
		InputSchema: json.RawMessage(listDevicesSchema),
	}}
	for _, t := range actionTools {
		if scope != nil && !scope.AllowsMethod(t.name) {
			continue
		}
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
