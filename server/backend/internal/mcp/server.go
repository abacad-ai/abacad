package mcp

import (
	"context"
	"encoding/json"
)

const (
	serverName    = "abacad"
	serverVersion = "0.2.0"
	// Protocol version we advertise if the client doesn't request one.
	defaultProtocolVersion = "2025-06-18"
)

// dispatch handles a single JSON-RPC request against the given resolver and
// returns the response, or nil for notifications (which get no reply).
func dispatch(ctx context.Context, req request, resolver DeviceResolver) *response {
	switch req.Method {
	case "initialize":
		resp := resultResponse(req.ID, initializeResult(req.Params))
		return &resp
	case "notifications/initialized":
		return nil // notification: no response
	case "ping":
		resp := resultResponse(req.ID, struct{}{})
		return &resp
	case "tools/list":
		resp := resultResponse(req.ID, map[string]any{"tools": toolInfos()})
		return &resp
	case "tools/call":
		resp := resultResponse(req.ID, callTool(ctx, req.Params, resolver))
		return &resp
	default:
		resp := errorResponse(req.ID, codeMethodNotFound, "method not found: "+req.Method)
		return &resp
	}
}

func initializeResult(params json.RawMessage) map[string]any {
	version := defaultProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &p); err == nil && p.ProtocolVersion != "" {
			version = p.ProtocolVersion // echo what the client asked for
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
	}
}

// callTool runs a tools/call request and returns a CallToolResult. Device-side
// failures (no device, timeout) come back as isError results — not JSON-RPC
// errors — so the agent sees a clean message and smoke.mjs can retry.
func callTool(ctx context.Context, params json.RawMessage, resolver DeviceResolver) toolResult {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResult("invalid tools/call params: " + err.Error())
	}

	if p.Name == listDevicesName {
		devices, err := resolver.List(ctx)
		if err != nil {
			return errorResult(err.Error())
		}
		out, _ := json.MarshalIndent(devices, "", "  ")
		return textResult(string(out))
	}

	tool, ok := actionByName[p.Name]
	if !ok {
		return errorResult("unknown tool: " + p.Name)
	}

	var sel deviceIDArg
	if len(p.Arguments) > 0 {
		_ = json.Unmarshal(p.Arguments, &sel)
	}
	dc, err := resolver.Resolve(ctx, sel.DeviceID)
	if err != nil {
		return errorResult(err.Error())
	}
	return tool.call(ctx, dc, p.Arguments)
}
