// Package protocol is the wire contract between the server and a device
// (starting with the Android app), spoken over the /device WebSocket.
//
// Ported verbatim from the v0 TypeScript server (server/src/protocol.ts) so the
// device side needs no change: the agent drives the device the way a human would
// — look at the screen, touch it, type, press the nav keys. Power is the device's
// own affair (its display timeout sleeps it; the app auto-wakes on the next
// command), so there are no wake/sleep methods here.
package protocol

import "encoding/json"

// Method is one of the fixed device operations. Kept in sync with the Android
// executor and the MCP tool surface.
type Method string

const (
	MethodScreenshot Method = "screenshot"
	MethodTap        Method = "tap"
	MethodLongPress  Method = "long_press"
	MethodSwipe      Method = "swipe"
	MethodInputText  Method = "input_text"
	MethodBack       Method = "back"
	MethodHome       Method = "home"
	MethodRecents    Method = "recents"

	// Desktop methods. A device implements the subset it supports; anything it
	// doesn't answer comes back as a device-side "unknown method" error, so the
	// tool surface can stay a superset without per-platform filtering.
	MethodClick      Method = "click"
	MethodRightClick Method = "right_click"
	MethodDrag       Method = "drag"
	MethodScroll     Method = "scroll"
	MethodPressKeys  Method = "press_keys"
	MethodComposite  Method = "composite"

	// Browser method. A browser device runs the semantic verbs
	// (screenshot/click/scroll/input_text) against its content iframe, plus
	// `execute` — the escape hatch that evaluates JavaScript in that surface.
	// Non-browser devices reject it as an unknown method.
	MethodExecute Method = "execute"
)

// Command is server -> device. id correlates the reply.
type Command struct {
	ID     string         `json:"id"`
	Method Method         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

// Reply is device -> server. Result is left as raw JSON and decoded by the
// caller into the method-specific shape below.
type Reply struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// UITreeNode is one element of the on-screen accessibility tree.
type UITreeNode struct {
	Cls       string `json:"cls"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	Clickable bool   `json:"clickable"`
	Bounds    [4]int `json:"bounds"` // [left, top, right, bottom]
}

// UITree is the accessibility tree delivered alongside a screenshot.
type UITree struct {
	Pkg   string       `json:"pkg"`
	Nodes []UITreeNode `json:"nodes"`
}

// ScreenshotResult is the result of a screenshot command.
type ScreenshotResult struct {
	W        int     `json:"w"`
	H        int     `json:"h"`
	PNGBase64 string `json:"png_base64"`
	Tree     *UITree `json:"tree,omitempty"` // present when include_ui_tree was true
}

// GestureResult is reported by tap / long_press / swipe.
type GestureResult struct {
	Dispatched bool `json:"dispatched"`
}

// InputTextResult is reported by input_text.
type InputTextResult struct {
	Set bool `json:"set"`
}

// GlobalActionResult is reported by back / home / recents.
type GlobalActionResult struct {
	Performed bool `json:"performed"`
}

// KeyResult is reported by press_keys.
type KeyResult struct {
	Pressed bool `json:"pressed"`
}

// CompositeResult is reported by composite: any screenshot steps in the sequence
// return their frames here, in step order. Empty when the sequence took no shots.
type CompositeResult struct {
	Shots []ScreenshotResult `json:"shots"`
}

// ExecuteResult is reported by execute: the JSON-serialized return value of the
// evaluated JavaScript. Value is null/absent when the code returned undefined.
// A thrown exception comes back as a failed Reply (ok:false, error) instead, so
// the agent sees it as a tool error rather than a value.
type ExecuteResult struct {
	Value json.RawMessage `json:"value,omitempty"`
}
