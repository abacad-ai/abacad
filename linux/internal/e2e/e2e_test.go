//go:build e2e

// Package e2e drives the whole Linux client end-to-end under a virtual X server
// (Xvfb): a mock /device relay sends each command verb over a real WebSocket and
// asserts the replies, and exercises the binary tunnel lane against a loopback
// echo server. Run with: make xvfb-test  (or: go test -tags e2e ./internal/e2e)
package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

	// --- mock relay: /device is the command+tunnel socket; /blobs is the data
	// plane the file-transfer verbs move bytes over. The client derives the
	// /blobs base from the ws URL's host, so both must live on this one server. ---
	connCh := make(chan *websocket.Conn, 1)
	textCh := make(chan string, 64)
	binCh := make(chan []byte, 64)
	blobs := newMockBlobs()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/blobs") {
			blobs.serveHTTP(w, r)
			return
		}
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

	// Positive proof injected motion reached the server: with humanize OFF the
	// pointer warps to the exact click point (the humanized path deliberately
	// jitters the landing, so the precise-coordinate check uses humanize:false).
	send("clickexact", "click", map[string]any{"x": 200, "y": 150, "humanize": false})
	if px, py, perr := xc.PointerPos(); perr == nil {
		ok("click(exact) warped pointer", px == 200 && py == 150, jsonStr(map[string]any{"x": px, "y": py}))
	} else {
		ok("click(exact) warped pointer", false, perr.Error())
	}

	// Humanized proof: from a parked cursor, a humanized click lands NEAR the
	// target (jittered, not exact) and takes human time — pre-action dwell +
	// per-step curved approach + press hold — whereas the teleport is instant.
	// That wall-clock gap is the behavioral signal the humanize path adds.
	send("park", "click", map[string]any{"x": 10, "y": 10, "humanize": false})
	tTele := time.Now()
	send("tele", "click", map[string]any{"x": 640, "y": 400, "humanize": false})
	teleDur := time.Since(tTele)
	send("park2", "click", map[string]any{"x": 10, "y": 10, "humanize": false})
	tHum := time.Now()
	send("hum", "click", map[string]any{"x": 640, "y": 400, "humanize": true})
	humDur := time.Since(tHum)
	if hx, hy, perr := xc.PointerPos(); perr == nil {
		ok("humanized click lands near target", absInt(hx-640) <= 30 && absInt(hy-400) <= 30,
			jsonStr(map[string]any{"x": hx, "y": hy}))
	} else {
		ok("humanized click lands near target", false, perr.Error())
	}
	ok("humanized click takes human time",
		humDur > 60*time.Millisecond && humDur > teleDur+40*time.Millisecond,
		fmt.Sprintf("humanized=%v teleport=%v", humDur, teleDur))

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

	// --- file transfer over the /blobs data plane (real HTTP round-trips) ---
	ftDir := t.TempDir()

	// pull_file: a file on disk -> a blob the agent can read. The device uploads
	// the bytes over HTTP; we assert the mock store received them intact.
	srcContent := bytes.Repeat([]byte("pull-me: the quick brown fox\n"), 500) // ~14 KB
	srcPath := filepath.Join(ftDir, "src.txt")
	if err := os.WriteFile(srcPath, srcContent, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	pull := send("pf", "pull_file", map[string]any{"src_path": srcPath})
	pr := result(pull)
	pulledID := str(pr["blob_id"])
	ok("pull_file ok", pull["ok"] == true && pulledID != "" && intOf(pr["size"]) == len(srcContent), jsonStr(pull))
	if got, has := blobs.get(pulledID); has {
		ok("pull_file bytes match", bytes.Equal(got, srcContent), "uploaded blob differs from source file")
	} else {
		ok("pull_file bytes match", false, "blob not in store")
	}
	pullSum := sha256.Sum256(srcContent)
	ok("pull_file sha256", str(pr["sha256"]) == hex.EncodeToString(pullSum[:]), jsonStr(pr))

	// push_file: a server-staged blob -> a file on the device disk. The device
	// downloads it over HTTP and writes it, applying the requested mode.
	pushContent := bytes.Repeat([]byte("push-me: lorem ipsum dolor\n"), 500)
	stagedID := blobs.put(pushContent)
	destPath := filepath.Join(ftDir, "sub", "dest.txt")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	push := send("ps", "push_file", map[string]any{"blob_id": stagedID, "dest_path": destPath, "mode": 0o600})
	pu := result(push)
	ok("push_file ok", push["ok"] == true && pu["written"] == true && intOf(pu["size"]) == len(pushContent), jsonStr(push))
	if got, err := os.ReadFile(destPath); err == nil {
		ok("push_file bytes match", bytes.Equal(got, pushContent), "written file differs from staged blob")
	} else {
		ok("push_file bytes match", false, err.Error())
	}
	if fi, err := os.Stat(destPath); err == nil {
		ok("push_file mode applied", fi.Mode().Perm() == 0o600, fi.Mode().String())
	} else {
		ok("push_file mode applied", false, err.Error())
	}

	// error path: pulling a file that isn't there fails cleanly (no panic, ok:false).
	miss := send("pm", "pull_file", map[string]any{"src_path": filepath.Join(ftDir, "nope.txt")})
	ok("pull_file missing rejected", miss["ok"] == false, jsonStr(miss))
	// error path: pushing an unknown blob id surfaces the 404 as an error.
	badPush := send("pb", "push_file", map[string]any{"blob_id": "blob_nope", "dest_path": filepath.Join(ftDir, "x.txt")})
	ok("push_file unknown blob rejected", badPush["ok"] == false, jsonStr(badPush))

	// --- screen recording (file channel): real ffmpeg x11grab -> mp4 -> /blobs ---
	if _, ffErr := exec.LookPath("ffmpeg"); ffErr != nil {
		t.Log("SKIP screen_recording: ffmpeg not on PATH")
	} else {
		start := send("sr1", "screen_recording", map[string]any{
			"action": "start", "file": map[string]any{"enabled": true, "fps": 10},
		})
		sr := result(start)
		ok("screen_recording start", start["ok"] == true && sr["state"] == "recording", jsonStr(start))
		ok("screen_recording dims", intOf(sr["width"]) == wantW && intOf(sr["height"]) == wantH, jsonStr(sr))

		time.Sleep(2 * time.Second) // capture a couple seconds of frames

		stop := send("sr2", "screen_recording", map[string]any{"action": "stop"})
		ok("screen_recording stop accepted", stop["ok"] == true, jsonStr(stop))

		// The transfer is async: poll status until it settles ready/failed.
		var blobID, state string
		for i := 0; i < 60; i++ {
			st := result(send(fmt.Sprintf("srs%d", i), "screen_recording", map[string]any{"action": "status"}))
			state = str(st["state"])
			if state == "ready" {
				blobID = str(st["blob_id"])
				break
			}
			if state == "failed" {
				t.Logf("screen_recording failed: %s", jsonStr(st))
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		ok("screen_recording ready", state == "ready" && blobID != "", "final state="+state)

		if data, has := blobs.get(blobID); has {
			ok("recording non-trivial size", len(data) > 1000, fmt.Sprintf("%d bytes", len(data)))
			head := data[:min(64, len(data))]
			ok("recording is mp4", bytes.Contains(head, []byte("ftyp")), fmt.Sprintf("head=%x", head))
		} else {
			ok("recording blob stored", false, "blob not in store: "+blobID)
		}
	}

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

// mockBlobs is a tiny in-memory stand-in for the server's /blobs data plane:
// POST stores bytes under a fresh id and returns {id,size,sha256}; GET streams
// them back. It lets the E2E exercise the device's real HTTP transfer path
// without standing up the whole backend.
type mockBlobs struct {
	mu sync.Mutex
	m  map[string][]byte
	n  int
}

func newMockBlobs() *mockBlobs { return &mockBlobs{m: map[string][]byte{}} }

func (b *mockBlobs) put(data []byte) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n++
	id := fmt.Sprintf("blob_%d", b.n)
	b.m[id] = append([]byte(nil), data...)
	return id
}

func (b *mockBlobs) get(id string) ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.m[id]
	return d, ok
}

func (b *mockBlobs) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		id := b.put(body)
		sum := sha256.Sum256(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "size": len(body), "sha256": hex.EncodeToString(sum[:]),
		})
	case http.MethodGet:
		id := strings.TrimPrefix(r.URL.Path, "/blobs/")
		d, ok := b.get(id)
		if !ok {
			http.Error(w, "blob not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(d)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func intOf(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return -1
}
func str(v any) string { s, _ := v.(string); return s }
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func jsonStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
