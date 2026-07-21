//go:build e2e

// Package e2e drives the whole Linux client end-to-end under a virtual X server
// (Xvfb): a mock /device relay sends each command verb over a real WebSocket and
// asserts the replies, and exercises the binary tunnel lane against a loopback
// echo server. Run with: make xvfb-test  (or: go test -tags e2e ./internal/e2e)
package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"abacad-linux/internal/agent"
	"abacad-linux/internal/x11"
)

const display = ":99"

func TestXvfbE2E(t *testing.T) {
	// --- virtual X server ---
	xvfb := exec.Command("Xvfb", display, "-screen", "0", "1280x800x24", "-nolisten", "tcp")
	if err := xvfb.Start(); err != nil {
		t.Skipf("Xvfb not available: %v", err)
	}
	defer func() { _ = xvfb.Process.Kill(); _, _ = xvfb.Process.Wait() }()
	os.Setenv("DISPLAY", display)

	var xc *x11.Conn
	for i := 0; i < 50; i++ {
		if c, err := x11.Open(); err == nil {
			xc = c
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if xc == nil {
		t.Fatal("X server did not come up")
	}
	defer xc.Close()
	wantW, wantH := xc.Size()
	if wantW != 1280 || wantH != 800 {
		t.Fatalf("unexpected geometry %dx%d", wantW, wantH)
	}

	// --- mock relay: accept the device socket, pump frames to channels ---
	connCh := make(chan *websocket.Conn, 1)
	textCh := make(chan string, 64)
	binCh := make(chan []byte, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		c.SetReadLimit(32 << 20)
		connCh <- c
		for {
			typ, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			if typ == websocket.MessageText {
				textCh <- string(data)
			} else {
				binCh <- append([]byte(nil), data...)
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/device"
	a, err := agent.New(wsURL, xc)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	go a.Run(ctx)

	var device *websocket.Conn
	select {
	case device = <-connCh:
	case <-time.After(5 * time.Second):
		t.Fatal("device never connected to mock relay")
	}

	// send issues a command and returns the correlated reply.
	send := func(id, method string, params map[string]any) map[string]any {
		t.Helper()
		cmd := map[string]any{"id": id, "method": method}
		if params != nil {
			cmd["params"] = params
		}
		b, _ := json.Marshal(cmd)
		if err := device.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatalf("write %s: %v", method, err)
		}
		select {
		case raw := <-textCh:
			var rep map[string]any
			if err := json.Unmarshal([]byte(raw), &rep); err != nil {
				t.Fatalf("bad reply json for %s: %v", method, err)
			}
			if rep["id"] != id {
				t.Fatalf("%s reply id = %v, want %s", method, rep["id"], id)
			}
			return rep
		case <-time.After(10 * time.Second):
			t.Fatalf("no reply for %s", method)
			return nil
		}
	}

	pass, fail := 0, 0
	ok := func(name string, cond bool, detail string) {
		if cond {
			pass++
			t.Logf("PASS %s", name)
		} else {
			fail++
			t.Errorf("FAIL %s: %s", name, detail)
		}
	}
	result := func(rep map[string]any) map[string]any {
		if m, isMap := rep["result"].(map[string]any); isMap {
			return m
		}
		return map[string]any{}
	}

	// --- screenshot + tree ---
	shot := send("1", "screenshot", map[string]any{"include_ui_tree": true})
	ok("screenshot ok", shot["ok"] == true, jsonStr(shot))
	res := result(shot)
	ok("screenshot dims", intOf(res["w"]) == wantW && intOf(res["h"]) == wantH, jsonStr(res))
	if b64, _ := res["png_base64"].(string); b64 != "" {
		raw, err := base64.StdEncoding.DecodeString(b64)
		img, derr := jpeg.Decode(bytes.NewReader(raw))
		ok("screenshot jpeg decodes", err == nil && derr == nil && img != nil, "decode error")
		if img != nil {
			bnd := img.Bounds()
			ok("jpeg geometry", bnd.Dx() == wantW && bnd.Dy() == wantH, bnd.String())
		}
	} else {
		ok("screenshot jpeg decodes", false, "no png_base64")
	}
	if tree, isMap := res["tree"].(map[string]any); isMap {
		nodes, _ := tree["nodes"].([]any)
		ok("tree present + empty (v1 stub)", len(nodes) == 0, jsonStr(tree))
	} else {
		ok("tree present + empty (v1 stub)", false, "no tree")
	}

	// --- input verbs: assert dispatched pipeline (no error) ---
	dispatched := func(name, method string, params map[string]any) {
		rep := send(name, method, params)
		r := result(rep)
		ok(name, rep["ok"] == true && r["dispatched"] == true, jsonStr(rep))
	}
	dispatched("tap", "tap", map[string]any{"x": 100, "y": 100})
	dispatched("click", "click", map[string]any{"x": 200, "y": 150})
	// Positive proof injected motion reached the server: the pointer warped to
	// the last click point (not just "the reply came back ok").
	if px, py, perr := xc.PointerPos(); perr == nil {
		ok("click warped pointer", px == 200 && py == 150, jsonStr(map[string]any{"x": px, "y": py}))
	} else {
		ok("click warped pointer", false, perr.Error())
	}
	dispatched("right_click", "right_click", map[string]any{"x": 200, "y": 150})
	dispatched("drag", "drag", map[string]any{"x1": 10, "y1": 10, "x2": 300, "y2": 200, "duration_ms": 40})
	dispatched("scroll", "scroll", map[string]any{"x": 400, "y": 400, "dy": 3})
	dispatched("long_press", "long_press", map[string]any{"x": 50, "y": 50, "duration_ms": 30})
	dispatched("swipe", "swipe", map[string]any{"x1": 10, "y1": 10, "x2": 100, "y2": 100, "duration_ms": 40})

	itext := send("it", "input_text", map[string]any{"text": "hello"})
	ok("input_text set", itext["ok"] == true && result(itext)["set"] == true, jsonStr(itext))

	// press_keys returning pressed:true proves the live keymap loaded and a
	// keycode resolved from the real X server.
	pk := send("pk", "press_keys", map[string]any{"keys": []any{"ctrl", "a"}})
	ok("press_keys pressed", pk["ok"] == true && result(pk)["pressed"] == true, jsonStr(pk))

	// composite with a screenshot step returns one shot.
	comp := send("cp", "composite", map[string]any{"steps": []any{
		map[string]any{"op": "wait", "ms": 1},
		map[string]any{"op": "click", "x": 5, "y": 5},
		map[string]any{"op": "screenshot"},
	}})
	shots, _ := result(comp)["shots"].([]any)
	ok("composite shots", comp["ok"] == true && len(shots) == 1, jsonStr(comp))

	// --- error paths ---
	back := send("bk", "back", nil)
	ok("back rejected", back["ok"] == false && strings.Contains(str(back["error"]), "no desktop analogue"), jsonStr(back))
	unk := send("uk", "frobnicate", nil)
	ok("unknown method", unk["ok"] == false && strings.Contains(str(unk["error"]), "unknown method"), jsonStr(unk))

	// --- tunnel lane against a loopback echo server ---
	ok("tunnel round-trip", tunnelRoundTrip(t, ctx, device, binCh), "see logs")

	t.Logf("E2E summary: %d passed, %d failed", pass, fail)
	if fail > 0 {
		t.Fatalf("%d checks failed", fail)
	}
}

// tunnelRoundTrip opens a stream to a loopback echo listener via the binary
// lane, sends bytes, and verifies they echo back, then closes.
func tunnelRoundTrip(t *testing.T, ctx context.Context, device *websocket.Conn, binCh chan []byte) bool {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Logf("tunnel: listen: %v", err)
		return false
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn) // echo
	}()

	const id = uint64(7)
	frame := func(typ byte, payload []byte) []byte {
		b := make([]byte, 9+len(payload))
		b[0] = typ
		binary.BigEndian.PutUint64(b[1:9], id)
		copy(b[9:], payload)
		return b
	}
	// Open (type 1) → Data (type 2) → expect echoed Data back.
	if err := device.Write(ctx, websocket.MessageBinary, frame(1, []byte(ln.Addr().String()))); err != nil {
		t.Logf("tunnel: open write: %v", err)
		return false
	}
	time.Sleep(200 * time.Millisecond)
	if err := device.Write(ctx, websocket.MessageBinary, frame(2, []byte("ping"))); err != nil {
		t.Logf("tunnel: data write: %v", err)
		return false
	}
	select {
	case f := <-binCh:
		got := f[9:]
		device.Write(ctx, websocket.MessageBinary, frame(3, nil)) // close
		return f[0] == 2 && string(got) == "ping"
	case <-time.After(5 * time.Second):
		t.Log("tunnel: no echo frame")
		return false
	}
}

func intOf(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return -1
}
func str(v any) string { s, _ := v.(string); return s }
func jsonStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
