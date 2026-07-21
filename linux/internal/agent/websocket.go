package agent

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// wsClient is the outbound WebSocket to the abacad relay's /device endpoint. The
// device dials out (NAT-friendly; the server never connects in). Text frames
// carry the JSON command/reply lane; binary frames carry the tunnel lane.
// Auto-reconnects with exponential backoff and pings the idle socket.
type wsClient struct {
	rawURL string
	token  string

	onText   func(string)
	onBinary func([]byte)
	onState  func(bool)

	mu     sync.Mutex
	conn   *websocket.Conn
	closed bool
}

// maxMessage lifts the read cap: relay screenshots are multi-MB base64.
const maxMessage = 32 * 1024 * 1024

// newWSClient validates and stores the endpoint. It refuses a non-ws/wss scheme
// and a plaintext (ws://) control channel to anything but loopback: this host
// carries screen contents and input injection, so a cleartext hop is a full
// MITM. The device token is pulled out of any ?token= and carried in the
// Authorization header instead, keeping it out of URLs and proxy logs.
func newWSClient(rawURL string) (*wsClient, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "ws" && scheme != "wss" {
		return nil, &configError{"server URL must be ws:// or wss://"}
	}
	if scheme == "ws" && !isLoopbackHost(u.Hostname()) {
		return nil, &configError{"refusing plaintext ws:// to a non-loopback host — use wss://"}
	}
	q := u.Query()
	token := q.Get("token")
	q.Del("token")
	u.RawQuery = q.Encode()
	return &wsClient{rawURL: u.String(), token: token}, nil
}

// run dials and services the socket until ctx is cancelled, reconnecting with
// exponential backoff (capped at 15s, matching the other clients).
func (w *wsClient) run(ctx context.Context) {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := w.serve(ctx); err != nil && ctx.Err() == nil {
			log.Printf("relay disconnected: %v (retry in %s)", err, backoff)
		}
		w.setState(false)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 15*time.Second {
			backoff = 15 * time.Second
		}
	}
}

// serve holds one connection: dial, pump reads, ping periodically. Returns when
// the connection drops.
func (w *wsClient) serve(ctx context.Context) error {
	header := http.Header{}
	if w.token != "" {
		header.Set("Authorization", "Bearer "+w.token)
	}
	conn, _, err := websocket.Dial(ctx, w.rawURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return err
	}
	conn.SetReadLimit(maxMessage)

	w.mu.Lock()
	w.conn = conn
	w.mu.Unlock()
	w.setState(true)
	log.Printf("relay connected")

	pingCtx, cancelPing := context.WithCancel(ctx)
	defer cancelPing()
	go w.pingLoop(pingCtx, conn)

	defer func() {
		conn.Close(websocket.StatusNormalClosure, "")
		w.mu.Lock()
		w.conn = nil
		w.mu.Unlock()
	}()

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		switch typ {
		case websocket.MessageText:
			if w.onText != nil {
				w.onText(string(data))
			}
		case websocket.MessageBinary:
			if w.onBinary != nil {
				w.onBinary(data)
			}
		}
	}
}

func (w *wsClient) pingLoop(ctx context.Context, conn *websocket.Conn) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_ = conn.Ping(pctx)
			cancel()
		}
	}
}

// sendText writes a JSON command reply on the command lane.
func (w *wsClient) sendText(s string) { w.write(websocket.MessageText, []byte(s)) }

// sendBinary writes a tunnel frame on the binary lane.
func (w *wsClient) sendBinary(b []byte) { w.write(websocket.MessageBinary, b) }

func (w *wsClient) write(typ websocket.MessageType, p []byte) {
	w.mu.Lock()
	conn := w.conn
	w.mu.Unlock()
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := conn.Write(ctx, typ, p); err != nil {
		log.Printf("send failed: %v", err)
	}
}

func (w *wsClient) setState(up bool) {
	if w.onState != nil {
		w.onState(up)
	}
}

func isLoopbackHost(host string) bool {
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// configError is a plain user-facing configuration error.
type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }
