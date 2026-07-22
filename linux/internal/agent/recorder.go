package agent

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"abacad-linux/internal/x11"
)

// recorder implements the screen_recording file channel on Linux by driving
// ffmpeg's x11grab: it records the primary display to a temp .mp4 at full
// resolution (H.264), then uploads it to /blobs on stop. It is the moving-picture
// counterpart of the JPEG screenshot.
//
// One recording at a time. The transfer is async — stop() flips to "uploading"
// and hands ffmpeg-finalize + upload to a goroutine, so a big clip never blocks
// the command window; the agent polls status() until the blob id appears. The
// temp file is removed after a successful upload (automatic retention). Video
// only (RFB carries no audio, so audio would be asymmetric across channels).
type recorder struct {
	mu sync.Mutex

	phase   string // idle | recording | uploading | ready | failed
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stderr  *bytes.Buffer
	path    string
	startAt time.Time

	width, height, fps int
	durationMs         int64
	sizeBytes          int64
	blobID, sha256     string
	errText            string

	x     *x11.Conn
	blobs *blobClient
}

func newRecorder(x *x11.Conn, blobs *blobClient) *recorder {
	return &recorder{phase: "idle", x: x, blobs: blobs}
}

// handle dispatches the screen_recording action. Called under the dispatcher
// lock; the recorder's own mutex guards state shared with the upload goroutine.
func (r *recorder) handle(params map[string]any) (map[string]any, error) {
	switch paramStr(params, "action", "") {
	case "start":
		file, _ := params["file"].(map[string]any)
		return r.start(file)
	case "stop":
		return r.stop(), nil
	case "status":
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.statusLocked(), nil
	default:
		return nil, fmt.Errorf(`screen_recording action must be "start", "stop", or "status"`)
	}
}

func (r *recorder) start(file map[string]any) (map[string]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.phase == "recording" {
		return nil, fmt.Errorf("a recording is already in progress; stop it first")
	}
	if r.blobs == nil {
		return nil, fmt.Errorf("screen recording needs the /blobs data plane, which is not configured on this device")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found on PATH — install ffmpeg to record the screen")
	}
	w, h := r.x.Size()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("screen has zero geometry (%dx%d)", w, h)
	}
	w &^= 1 // even dimensions for yuv420p / H.264
	h &^= 1
	fps := paramInt(file, "fps", 0)
	if fps <= 0 {
		fps = 30
	}
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}
	path := filepath.Join(os.TempDir(), fmt.Sprintf("abacad-rec-%d.mp4", time.Now().UnixNano()))

	cmd := exec.Command(ffmpeg,
		"-loglevel", "error", "-y",
		"-f", "x11grab",
		"-framerate", strconv.Itoa(fps),
		"-video_size", fmt.Sprintf("%dx%d", w, h),
		"-i", display,
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		path,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start ffmpeg: %w", err)
	}

	r.phase = "recording"
	r.cmd, r.stdin, r.stderr = cmd, stdin, &stderr
	r.path = path
	r.startAt = time.Now()
	r.width, r.height, r.fps = w, h, fps
	r.durationMs, r.sizeBytes = 0, 0
	r.blobID, r.sha256, r.errText = "", "", ""

	// Best-effort safety cap: auto-stop after max_duration_seconds.
	if capSecs := paramInt(file, "max_duration_seconds", 0); capSecs > 0 {
		go func() {
			time.Sleep(time.Duration(capSecs) * time.Second)
			r.stop() // no-op if already stopped
		}()
	}

	return map[string]any{"state": "recording", "width": w, "height": h, "fps": fps}, nil
}

// stop transitions to "uploading" and hands ffmpeg-finalize + upload to a
// goroutine, returning immediately. A stop with nothing recording is a no-op
// reporting the current state.
func (r *recorder) stop() map[string]any {
	r.mu.Lock()
	if r.phase != "recording" || r.cmd == nil {
		s := r.statusLocked()
		r.mu.Unlock()
		return s
	}
	cmd, stdin, stderr, path := r.cmd, r.stdin, r.stderr, r.path
	blobs := r.blobs
	r.cmd, r.stdin = nil, nil
	r.durationMs = time.Since(r.startAt).Milliseconds()
	r.phase = "uploading"
	r.mu.Unlock()

	go func() {
		// Ask ffmpeg to quit cleanly ("q") so it writes the moov atom, then wait.
		_, _ = io.WriteString(stdin, "q\n")
		_ = stdin.Close()
		waitErr := cmd.Wait()
		size := fileSize(path)

		r.mu.Lock()
		defer r.mu.Unlock()
		r.sizeBytes = size
		if size == 0 {
			r.phase = "failed"
			r.errText = "recording produced no data" + ffmpegErr(waitErr, stderr)
			os.Remove(path)
			return
		}
		id, _, sha, err := blobs.upload(path)
		if err != nil {
			r.phase = "failed"
			r.errText = err.Error()
			return
		}
		os.Remove(path) // auto-retention: keep only the store copy
		r.phase = "ready"
		r.blobID, r.sha256 = id, sha
	}()

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked()
}

// statusLocked renders the current state; the caller holds r.mu.
func (r *recorder) statusLocked() map[string]any {
	out := map[string]any{"state": r.phase}
	if r.width > 0 {
		out["width"] = r.width
	}
	if r.height > 0 {
		out["height"] = r.height
	}
	if r.fps > 0 {
		out["fps"] = r.fps
	}
	switch r.phase {
	case "recording":
		out["elapsed_ms"] = time.Since(r.startAt).Milliseconds()
		out["size_bytes"] = fileSize(r.path)
	case "uploading", "ready", "failed":
		out["duration_ms"] = r.durationMs
		out["size_bytes"] = r.sizeBytes
		out["codec"] = "h264"
		switch r.phase {
		case "ready":
			out["transfer_state"] = "ready"
		case "failed":
			out["transfer_state"] = "failed"
		default:
			out["transfer_state"] = "uploading"
		}
		if r.blobID != "" {
			out["blob_id"] = r.blobID
		}
		if r.sha256 != "" {
			out["sha256"] = r.sha256
		}
		if r.errText != "" {
			out["error"] = r.errText
		}
	}
	return out
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// ffmpegErr attaches a short reason (wait error + a stderr snippet) to a failure.
func ffmpegErr(waitErr error, stderr *bytes.Buffer) string {
	var parts []string
	if waitErr != nil {
		parts = append(parts, waitErr.Error())
	}
	if stderr != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			if len(s) > 200 {
				s = s[:200]
			}
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}
