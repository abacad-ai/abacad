package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"abacad/internal/version"
)

const (
	serverName = "abacad"
	// Protocol version we advertise if the client doesn't request one.
	defaultProtocolVersion = "2025-06-18"
)

// dispatch handles a single JSON-RPC request against the given resolver and
// scope, returning the response, or nil for notifications (which get no reply).
func dispatch(ctx context.Context, req request, resolver DeviceResolver, scope Scope, blobs BlobStore) *response {
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
		resp := resultResponse(req.ID, map[string]any{"tools": toolInfos(scope)})
		return &resp
	case "tools/call":
		resp := resultResponse(req.ID, callTool(ctx, req.Params, resolver, scope, blobs))
		return &resp
	default:
		resp := errorResponse(req.ID, codeMethodNotFound, "method not found: "+req.Method)
		return &resp
	}
}

func initializeResult(params json.RawMessage) map[string]any {
	protoVer := defaultProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &p); err == nil && p.ProtocolVersion != "" {
			protoVer = p.ProtocolVersion // echo what the client asked for
		}
	}
	return map[string]any{
		"protocolVersion": protoVer,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": serverName, "version": version.Version},
	}
}

// callTool runs a tools/call request and returns a CallToolResult. Device-side
// failures (no device, timeout) come back as isError results — not JSON-RPC
// errors — so the agent sees a clean message and smoke.mjs can retry.
func callTool(ctx context.Context, params json.RawMessage, resolver DeviceResolver, scope Scope, blobs BlobStore) toolResult {
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
	if scope != nil && !scope.AllowsMethod(p.Name) {
		return errorResult(fmt.Sprintf("method %q is not permitted for this API key", p.Name))
	}

	var sel deviceIDArg
	if len(p.Arguments) > 0 {
		_ = json.Unmarshal(p.Arguments, &sel)
	}
	dc, err := resolver.Resolve(ctx, sel.DeviceID)
	if err != nil {
		return errorResult(err.Error())
	}

	// File-transfer tools also need the caller's account and the blob store to
	// stage/read bytes on the agent's behalf; the plain device verbs don't.
	if tool.fileCall != nil {
		if blobs == nil {
			return errorResult("file transfer is not configured on this server")
		}
		return tool.fileCall(ctx, dc, p.Arguments, resolver.AccountID(), blobs)
	}
	return tool.call(ctx, dc, p.Arguments)
}
