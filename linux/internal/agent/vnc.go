package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// vncHandler runs the screen_recording live channel on Linux. On "start" it
// launches a localhost VNC server (x11vnc) for the X display and dials a dedicated
// WebSocket out to the server's ingress url, then pipes that WebSocket to the local
// VNC server: RFB bytes flow browser <-> server <-> this WS <-> x11vnc. The pixels
// ride this dedicated connection, never the command socket. One session at a time.
type vncHandler struct {
	mu     sync.Mutex
	active bool
	stopFn func()
}

func newVNCHandler() *vncHandler { return &vncHandler{} }

// startLocalVNC launches a localhost VNC server for the display and returns its
// 127.0.0.1 address and a stop func. It is a package var so tests can substitute a
// mock RFB server without needing x11vnc or a real X display.
var startLocalVNC = func(display string) (addr string, stop func(), err error) {
	if _, err := exec.LookPath("x11vnc"); err != nil {
		return "", nil, fmt.Errorf("x11vnc not found on PATH — install x11vnc for live view")
	}
	port, err := freePort()
	if err != nil {
		return "", nil, err
	}
	if display == "" {
		display = ":0"
	}
	cmd := exec.Command("x11vnc",
		"-display", display, "-localhost", "-rfbport", strconv.Itoa(port),
		"-nopw", "-forever", "-shared", "-quiet", "-noxdamage",
	)
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	stop = func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }
	addr = "127.0.0.1:" + strconv.Itoa(port)
	if err := waitListen(addr, 5*time.Second); err != nil {
		stop()
		return "", nil, fmt.Errorf("x11vnc did not start listening: %w", err)
	}
	return addr, stop, nil
}

func (v *vncHandler) handle(params map[string]any) (map[string]any, error) {
	switch paramStr(params, "action", "") {
	case "start":
		return v.start(paramStr(params, "url", ""))
	case "stop":
		v.stop()
		return map[string]any{"stopped": true}, nil
	default:
		return nil, fmt.Errorf(`vnc action must be "start" or "stop"`)
	}
}

func (v *vncHandler) start(url string) (map[string]any, error) {
	if url == "" {
		return nil, fmt.Errorf("vnc start requires url")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active {
		v.stopLocked()
	}

	addr, stopServer, err := startLocalVNC(os.Getenv("DISPLAY"))
	if err != nil {
		return nil, fmt.Errorf("start vnc server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		cancel()
		stopServer()
		return nil, fmt.Errorf("dial vnc ingress: %w", err)
	}
	ws.SetReadLimit(16 << 20)

	tcp, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		cancel()
		_ = ws.Close(websocket.StatusInternalError, "no local vnc")
		stopServer()
		return nil, fmt.Errorf("connect local vnc: %w", err)
	}

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			_ = ws.Close(websocket.StatusNormalClosure, "vnc stopped")
			_ = tcp.Close()
			stopServer()
		})
	}
	v.active = true
	v.stopFn = stop

	go func() {
		pipeWSTCP(ctx, ws, tcp)
		v.stop() // either side closed → tear the session down
	}()

	return map[string]any{"started": true}, nil
}

func (v *vncHandler) stop() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.stopLocked()
}

func (v *vncHandler) stopLocked() {
	if v.stopFn != nil {
		v.stopFn()
		v.stopFn = nil
	}
	v.active = false
}

// pipeWSTCP relays bytes between the ingress WebSocket and the local VNC TCP
// socket, both ways, until either side errors. RFB is an ordered byte stream, so
// WS message boundaries don't matter — only order, which both directions preserve.
func pipeWSTCP(ctx context.Context, ws *websocket.Conn, tcp net.Conn) {
	done := make(chan struct{}, 2)
	go func() { // WS -> TCP (viewer input)
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				done <- struct{}{}
				return
			}
			if _, err := tcp.Write(data); err != nil {
				done <- struct{}{}
				return
			}
		}
	}()
	go func() { // TCP -> WS (framebuffer)
		buf := make([]byte, 32<<10)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				if werr := ws.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					done <- struct{}{}
					return
				}
			}
			if err != nil {
				done <- struct{}{}
				return
			}
		}
	}()
	<-done
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func waitListen(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}
