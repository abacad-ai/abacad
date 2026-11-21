// Package mcpserver exposes the one AI-facing tool abacad needs: a list of
// devices with a standard VNC endpoint. The agent brings its own VNC client;
// there is no screenshot/click protocol to learn.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"abacad/internal/config"
	"abacad/internal/inventory"
)

type Server struct {
	inv     *inventory.Inventory
	host    string
	baseURL string
	version string
}

func New(inv *inventory.Inventory, host, baseURL, version string) *Server {
	return &Server{inv: inv, host: host, baseURL: baseURL, version: version}
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
	DirectHost string `json:"direct_host,omitempty"`
	DirectPort int    `json:"direct_port,omitempty"`
	BrowserURL string `json:"browser_url,omitempty"`
	Banner     string `json:"banner,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

type listOut struct {
	Devices []deviceOut `json:"devices"`
}

// Run serves MCP over stdio until the client disconnects.
func (s *Server) Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "abacad", Version: s.version}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_devices",
		Description: "List VNC devices on this LAN and the standard VNC endpoint to use. " +
			"Connect with any VNC client to vnc_host:vnc_port and use token as the VNC password. " +
			"Devices without a vnc_port are browser-only; open browser_url instead.",
	}, s.listDevices)
	return server.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) listDevices(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	devices := s.inv.Devices()
	out := listOut{Devices: make([]deviceOut, 0, len(devices))}
	for _, d := range devices {
		item := deviceOut{
			ID:         d.ID,
			Name:       d.Name,
			Platform:   d.Platform,
			Online:     d.Online,
			Mode:       d.Mode,
			Banner:     d.Banner,
			LastError:  d.LastError,
			BrowserURL: fmt.Sprintf("%s/vnc/%s", s.baseURL, d.ID),
		}
		switch {
		case d.Mode == config.ModeDirect:
			item.Auth = "device"
			item.DirectHost = d.Host
			item.DirectPort = d.Port
		case d.VNCPort > 0 && d.Mode == config.ModeL4:
			item.Auth = "device"
			item.VNCHost = s.host
			item.VNCPort = d.VNCPort
		case d.VNCPort > 0:
			item.Auth = "hub-token"
			item.VNCHost = s.host
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
