// Package mcpserver exposes the one AI-facing tool abacad needs: a list of
// devices with a standard VNC endpoint. The agent brings its own VNC client;
// there is no screenshot/click protocol to learn.
package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
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
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		return s.server(scheme + "://" + r.Host)
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

func (s *Server) server(baseURL string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "abacad", Version: s.version}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_devices",
		Description: "List the VNC devices configured in this hub. Each device has a ws_url: a " +
			"websockify-style WebSocket carrying plain RFB. Authenticate with the same key you use " +
			"for this MCP server, as an Authorization: Bearer header or a ?token= query parameter. " +
			"Use a WebSocket VNC client (noVNC, py-wsvnc) or bridge it to a local TCP port with " +
			"websocat for any VNC client. auth tells you whether the VNC handshake needs a password.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return s.listDevices(ctx, baseURL)
	})
	return srv
}

type deviceOut struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform,omitempty"`
	Online     bool   `json:"online"`
	Known      bool   `json:"known"`
	Status     string `json:"status"`
	Mode       string `json:"mode"`
	WSURL      string `json:"ws_url,omitempty"`
	Auth       string `json:"auth,omitempty"`
	SignIn     string `json:"sign_in,omitempty"`
	Username   string `json:"username,omitempty"`
	DirectHost string `json:"direct_host,omitempty"`
	DirectPort int    `json:"direct_port,omitempty"`
	BrowserURL string `json:"browser_url,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

type listOut struct {
	Devices []deviceOut `json:"devices"`
}

func deviceStatus(known, online bool) string {
	if !known {
		return "unknown"
	}
	if online {
		return "online"
	}
	return "offline"
}

func (s *Server) listDevices(ctx context.Context, baseURL string) (*mcp.CallToolResult, any, error) {
	wsBase := "ws" + strings.TrimPrefix(baseURL, "http")
	devices := s.inv.Devices()
	out := listOut{Devices: make([]deviceOut, 0, len(devices))}
	for _, d := range devices {
		item := deviceOut{
			ID:        d.ID,
			Name:      d.Name,
			Platform:  d.Platform,
			Online:    d.Online,
			Known:     d.Known,
			Status:    deviceStatus(d.Known, d.Online),
			Mode:      d.Mode,
			SignIn:    d.Auth,
			Username:  d.Username,
			LastError: d.LastError,
		}
		switch d.Mode {
		case config.ModeDirect:
			item.Auth = "device"
			item.DirectHost = d.Host
			item.DirectPort = d.Port
		case config.ModeL4:
			// The hub pipes bytes blindly; the client answers the device's
			// own VNC password.
			item.Auth = "device"
			item.WSURL = fmt.Sprintf("%s/ws/vnc/%s", wsBase, d.ID)
			item.BrowserURL = fmt.Sprintf("%s/vnc/%s", baseURL, d.ID)
		default:
			// The hub signs in to the device; the client sees no password.
			item.Auth = "none"
			item.WSURL = fmt.Sprintf("%s/ws/vnc/%s", wsBase, d.ID)
			item.BrowserURL = fmt.Sprintf("%s/vnc/%s", baseURL, d.ID)
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
