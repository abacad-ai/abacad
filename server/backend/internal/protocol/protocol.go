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
	// (screenshot/click/scroll/input_text) against its own page, plus
	// `execute` — the escape hatch that evaluates JavaScript in that page.
	// Non-browser devices reject it as an unknown method.
	MethodExecute Method = "execute"

	// File-transfer methods bridge the device's filesystem to the /blobs data
	// plane. The bytes ride HTTP (the device fetches/posts /blobs with its own
	// token), never this WebSocket — the frame only carries the blob id and the
	// on-device path. A device without a filesystem verb rejects them as unknown.
	MethodPushFile Method = "push_file" // server-staged blob -> device file
	MethodPullFile Method = "pull_file" // device file -> blob the agent can read

	// Screen recording: a continuous on-device capture, written to a local file at
	// full quality and uploaded to /blobs on stop — the moving-picture counterpart
	// of screenshot. One method carries the whole lifecycle via an "action" param
	// (start|stop|status), mirroring the single screen_recording MCP tool. The bytes
	// ride the /blobs data plane (the device uploads on stop), never this socket.
	// The live (VNC) observation channel is a separate future addition.
	MethodScreenRecording Method = "screen_recording"
)

// Methods is the full set of device methods, in MCP-tool order. It is the source
// of truth for validating an API key's method allowlist (list_devices, being
// metadata rather than a device operation, is not included).
var Methods = []Method{
	MethodScreenshot, MethodTap, MethodLongPress, MethodSwipe, MethodInputText,
	MethodBack, MethodHome, MethodRecents,
	MethodClick, MethodRightClick, MethodDrag, MethodScroll, MethodPressKeys, MethodComposite,
	MethodExecute,
	MethodPushFile, MethodPullFile,
	MethodScreenRecording,
}

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

// Activity is a device's coarse power state, reported by the device itself. The
// server cannot infer it: a socket kept alive through sleep (the device holds a
// wakelock so its pings keep flowing) is byte-for-byte identical to an awake
// one. "active" = screen interactive; "asleep" = screen off but still connected
// and reachable (a command auto-wakes it). Unknown/absent is treated as active.
type Activity string

const (
	ActivityActive Activity = "active"
	ActivityAsleep Activity = "asleep"
)

// Presence is an unsolicited device -> server frame reporting a change in the
// device's power state. Unlike a Reply it has no id — the server distinguishes
// it by the "type":"presence" tag before the reply path. Servers that predate
// this frame ignore it (no matching pending id), so a device may send it to any
// server version safely.
type Presence struct {
	Type  string   `json:"type"` // always "presence"
	State Activity `json:"state"`
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
	W         int     `json:"w"`
	H         int     `json:"h"`
	PNGBase64 string  `json:"png_base64"`
	Tree      *UITree `json:"tree,omitempty"` // present when include_ui_tree was true
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

// PushFileResult is reported by push_file: the device downloaded the staged blob
// and wrote it to the target path. Size and SHA256 are what the device actually
// wrote, so the server can compare them against the staged blob to confirm the
// bytes arrived intact.
type PushFileResult struct {
	Written bool   `json:"written"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"` // hex
}

// PullFileResult is reported by pull_file: the device read the source file and
// uploaded it to /blobs (under the same account), returning the blob id the
// agent can then read. Size/SHA256 describe the uploaded bytes.
type PullFileResult struct {
	BlobID string `json:"blob_id"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"` // hex
}

// ScreenRecordingResult is reported by screen_recording for every action. Fields
// are populated as the action and state warrant:
//
//   - start  → State = "recording".
//   - status → State, plus ElapsedMs / SizeBytes while recording or transferring;
//     once the finished file has uploaded, TransferState = "ready" and BlobID set.
//   - stop   → the finalized file's metadata (Width/Height/FPS/DurationMs/SizeBytes/
//     Codec) with TransferState = "uploading"; the upload runs in the background,
//     so the agent polls status until TransferState = "ready" and BlobID appears.
//
// State walks idle → recording → stopped → uploading → ready (or failed). The
// transfer is async because a full-quality clip can be far larger than the
// command window; only the blob id and metadata ever cross this socket — the bytes
// go device → /blobs over HTTP, and the agent fetches them from GET /blobs/{id}.
type ScreenRecordingResult struct {
	State         string `json:"state"`                 // idle | recording | stopped | uploading | ready | failed
	ElapsedMs     int64  `json:"elapsed_ms,omitempty"`  // wall time recorded so far (status while recording)
	DurationMs    int64  `json:"duration_ms,omitempty"` // final clip length (stop/ready)
	Width         int    `json:"width,omitempty"`       // captured pixel dimensions
	Height        int    `json:"height,omitempty"`
	FPS           int    `json:"fps,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	Codec         string `json:"codec,omitempty"`          // e.g. "h264"
	TransferState string `json:"transfer_state,omitempty"` // "" | uploading | ready | failed
	BlobID        string `json:"blob_id,omitempty"`        // set once uploaded; fetch GET /blobs/{id}
	SHA256        string `json:"sha256,omitempty"`         // hex, of the uploaded bytes
	Error         string `json:"error,omitempty"`          // failure detail when State/TransferState = failed
}
