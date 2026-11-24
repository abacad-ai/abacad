package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"abacad/internal/config"
	"abacad/internal/gateway"
	"abacad/internal/inventory"
	mcpserver "abacad/internal/mcp"
)

func TestLoginAndAuth(t *testing.T) {
	srv, _ := newTestServer(t, "")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/devices")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/devices = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	body := bytes.NewBufferString(`{"token":"test-admin"}`)
	resp, err = http.Post(srv.URL+"/api/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	cookies := resp.Cookies()
	_ = resp.Body.Close()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/devices", nil)
	req.AddCookie(cookies[0])
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated /api/devices = %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["devices"]; !ok {
		t.Fatalf("unexpected response %v", out)
	}
}

func TestConsoleRoute(t *testing.T) {
	srv, _ := newTestServer(t, "")
	defer srv.Close()

	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := noRedirect.Get(srv.URL + "/devices/mock")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("signed-out /devices/mock = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/?next=/devices/mock" {
		t.Fatalf("redirect = %q", got)
	}

	login, err := http.Post(srv.URL+"/api/login", "application/json", strings.NewReader(`{"token":"test-admin"}`))
	if err != nil {
		t.Fatal(err)
	}
	cookies := login.Cookies()
	_ = login.Body.Close()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	req, _ := http.NewRequest("GET", srv.URL+"/devices/mock", nil)
	req.AddCookie(cookies[0])
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Skip("frontend not built")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signed-in /devices/mock = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content type = %q", ct)
	}
}

func TestWebSocketBrowserPath(t *testing.T) {
	addr, port, stop := startMockVNC(t)
	defer stop()
	srv, _ := newTestServer(t, fmt.Sprintf("%s", addr))
	defer srv.Close()
	_ = port

	client := &http.Client{}
	resp, err := client.Post(srv.URL+"/api/login", "application/json", strings.NewReader(`{"token":"test-admin"}`))
	if err != nil {
		t.Fatal(err)
	}
	cookies := resp.Cookies()
	_ = resp.Body.Close()
	if len(cookies) == 0 {
		t.Fatal("no cookie")
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/vnc/mock"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader:   http.Header{"Cookie": []string{cookies[0].Name + "=" + cookies[0].Value}},
		Subprotocols: []string{"binary"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(websocket.StatusNormalClosure, "done")
	c.SetReadLimit(1 << 20)

	r := &wsReader{c: c, ctx: ctx}
	write := func(b []byte) {
		if err := c.Write(ctx, websocket.MessageBinary, b); err != nil {
			t.Fatal(err)
		}
	}
	read := func(n int) []byte {
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			t.Fatal(err)
		}
		return b
	}

	banner := read(12)
	if string(banner) != "RFB 003.008\n" {
		t.Fatalf("banner = %q", banner)
	}
	write([]byte("RFB 003.008\n"))
	head := read(2)
	if head[0] != 1 || head[1] != 1 {
		t.Fatalf("expected one None security type, got %v", head)
	}
	write([]byte{1})
	result := read(4)
	if binary.BigEndian.Uint32(result) != 0 {
		t.Fatalf("SecurityResult = %d", binary.BigEndian.Uint32(result))
	}
	write([]byte{1}) // ClientInit shared
	hdr := read(24)
	nameLen := binary.BigEndian.Uint32(hdr[20:24])
	_ = read(int(nameLen))
	write([]byte("hello"))
	if got := string(read(5)); got != "hello" {
		t.Fatalf("echo = %q", got)
	}
}

type wsReader struct {
	c   *websocket.Conn
	ctx context.Context
	buf []byte
}

func (r *wsReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		typ, data, err := r.c.Read(r.ctx)
		if err != nil {
			return 0, err
		}
		if typ != websocket.MessageBinary {
			continue
		}
		r.buf = data
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func newTestServer(t *testing.T, upstreamAddr string) (*httptest.Server, *inventory.Inventory) {
	t.Helper()
	cfg, err := config.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Listen = "127.0.0.1:0"
	cfg.AdminToken = "test-admin"
	cfg.CookieSecret = "test-cookie-secret"
	cfg.Devices = []config.Device{}
	if upstreamAddr != "" {
		host, p, err := net.SplitHostPort(upstreamAddr)
		if err != nil {
			t.Fatal(err)
		}
		var port int
		_, _ = fmt.Sscanf(p, "%d", &port)
		cfg.Devices = append(cfg.Devices, config.Device{
			ID: "mock", Name: "Mock", Host: host, Port: port,
			Auth: config.AuthVNC, Password: "device-pass", Mode: config.ModeGateway, Token: "Abcd1234",
		})
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(cfg)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	gw := gateway.New(inv, log)
	gw.Sync(inv.Devices())
	mcpSrv := mcpserver.New(inv, "test")
	s := New(inv, gw, mcpSrv, log, "test")
	return httptest.NewServer(s.Handler()), inv
}

func startMockVNC(t *testing.T) (addr string, port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go mockVNCServer(conn)
		}
	}()
	return ln.Addr().String(), ln.Addr().(*net.TCPAddr).Port, func() { _ = ln.Close() }
}

func mockVNCServer(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	fmt.Fprint(conn, "RFB 003.008\n")
	if _, err := br.ReadString('\n'); err != nil {
		return
	}
	conn.Write([]byte{2, 1, 2}) // two types: None, VNC auth
	sel, err := br.ReadByte()
	if err != nil {
		return
	}
	if sel == 2 {
		challenge := []byte("0123456789abcdef")
		conn.Write(challenge)
		resp := make([]byte, 16)
		if _, err := io.ReadFull(br, resp); err != nil {
			return
		}
		// The mock does not verify the password; the hub handshake is what is
		// under test here.
	}
	binary.Write(conn, binary.BigEndian, uint32(0))
	if _, err := br.ReadByte(); err != nil { // ClientInit
		return
	}
	hdr := make([]byte, 24)
	binary.BigEndian.PutUint16(hdr[0:2], 800)
	binary.BigEndian.PutUint16(hdr[2:4], 600)
	hdr[4] = 32
	hdr[5] = 24
	hdr[7] = 1
	binary.BigEndian.PutUint16(hdr[8:10], 255)
	binary.BigEndian.PutUint16(hdr[10:12], 255)
	binary.BigEndian.PutUint16(hdr[12:14], 255)
	hdr[14] = 16
	hdr[15] = 8
	binary.BigEndian.PutUint32(hdr[20:24], 4)
	conn.Write(hdr)
	conn.Write([]byte("mock"))
	buf := make([]byte, 256)
	for {
		n, err := br.Read(buf)
		if n > 0 {
			conn.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func TestMCPHTTP(t *testing.T) {
	srv, _ := newTestServer(t, "")
	defer srv.Close()

	post := func(body string) *http.Response {
		req, err := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer test-admin")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	resp := post(initBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !bytes.Contains(b, []byte(`"serverInfo"`)) {
		t.Fatalf("initialize response: %s", b)
	}

	// Missing bearer token must be rejected before MCP sees the request.
	req, _ := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /mcp = %d", resp.StatusCode)
	}
}

type bearerTransport struct{ token string }

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(req)
}

func TestMCPOfficialClient(t *testing.T) {
	srv, _ := newTestServer(t, "")
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:   srv.URL + "/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{token: "test-admin"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_devices", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok || !strings.Contains(text.Text, "devices") {
		t.Fatalf("content: %+v", res.Content[0])
	}
}
