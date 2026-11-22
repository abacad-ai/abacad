// Package mcpserver exposes the one AI-facing tool abacad needs: a list of
// devices with a standard VNC endpoint. The agent brings its own VNC client;
// there is no screenshot/click protocol to learn.
package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"abacad/internal/config"
	"abacad/internal/inventory"
)

type Server struct {
	inv     *inventory.Inventory
	version string
}

func New(inv *inventory.Inventory, version string) *Server {
	return &Server{inv: inv, version: version}
}

// HTTPHandler serves MCP Streamable HTTP at /mcp. It returns JSON rather than
// SSE responses (the client may still open the optional standalone SSE stream).
// Authentication is a Bearer admin token, deliberately separate from the
// browser cookie.
func (s *Server) HTTPHandler() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		return s.server(host, scheme+"://"+r.Host)
	}, &mcp.StreamableHTTPOptions{
		JSONResponse:   true,
		SessionTimeout: 30 * time.Minute,
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !bearerOK(r, s.inv.Config().AdminToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="abacad"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func bearerOK(r *http.Request, token string) bool {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(h[len(prefix):])), []byte(token)) == 1
}

func (s *Server) server(host, baseURL string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "abacad", Version: s.version}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_devices",
		Description: "List VNC devices on this LAN and the standard VNC endpoint to use. " +
			"Connect with any VNC client to vnc_host:vnc_port and use token as the VNC password. " +
			"Devices without a vnc_port are browser-only; open browser_url instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return s.listDevices(ctx, host, baseURL)
	})
	return srv
}

type deviceOut struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform,omitempty"`
	Online     bool   `json:"online"`
	Mode       string `json:"mode"`
	VNCHost    string `json:"vnc_host,omitempty"`
	VNCPort    int    `json:"vnc_port,omitempty"`
	Token      string `json:"token,omitempty"`
	Auth       string `json:"auth,omitempty"`
	SignIn     string `json:"sign_in,omitempty"`
	Username   string `json:"username,omitempty"`
	DirectHost string `json:"direct_host,omitempty"`
	DirectPort int    `json:"direct_port,omitempty"`
	BrowserURL string `json:"browser_url,omitempty"`
	Banner     string `json:"banner,omitempty"`
	Warning    string `json:"warning,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

type listOut struct {
	Devices []deviceOut `json:"devices"`
}

func (s *Server) listDevices(ctx context.Context, host, baseURL string) (*mcp.CallToolResult, any, error) {
	devices := s.inv.Devices()
	out := listOut{Devices: make([]deviceOut, 0, len(devices))}
	for _, d := range devices {
		item := deviceOut{
			ID:        d.ID,
			Name:      d.Name,
			Platform:  d.Platform,
			Online:    d.Online,
			Mode:      d.Mode,
			SignIn:    d.Auth,
			Username:  d.Username,
			Banner:    d.Banner,
			Warning:   d.Warning,
			LastError: d.LastError,
		}
		if d.Mode != config.ModeDirect {
			item.BrowserURL = fmt.Sprintf("%s/vnc/%s", baseURL, d.ID)
		}
		switch {
		case d.Mode == config.ModeDirect:
			item.Auth = "device"
			item.DirectHost = d.Host
			item.DirectPort = d.Port
		case d.VNCPort > 0 && d.Mode == config.ModeL4:
			item.Auth = "device"
			item.VNCHost = host
			item.VNCPort = d.VNCPort
		case d.VNCPort > 0:
			item.Auth = "hub-token"
			item.VNCHost = host
			item.VNCPort = d.VNCPort
			item.Token = d.Token
		}
		out.Devices = append(out.Devices, item)
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}
