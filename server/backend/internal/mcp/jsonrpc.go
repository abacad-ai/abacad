// Package mcp implements a minimal, stateless MCP server over HTTP (Streamable
// HTTP transport) — just enough JSON-RPC 2.0 to satisfy the official MCP client
// used by real agents (Claude Code) and by smoke.mjs: initialize, tools/list,
// tools/call, and the notifications/initialized no-op.
//
// Hand-rolled rather than pulling an SDK: the surface is tiny and this keeps the
// auth -> account -> device-resolution boundary fully in our hands.
package mcp

import "encoding/json"

// jsonRPCVersion is the only version we speak.
const jsonRPCVersion = "2.0"

// request is an incoming JSON-RPC message. id is absent for notifications.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r *request) isNotification() bool { return len(r.ID) == 0 }

// response is an outgoing JSON-RPC reply. Exactly one of Result / Error is set.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC error codes we use.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

func resultResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: jsonRPCVersion, ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, msg string) response {
	if id == nil {
		id = json.RawMessage("null")
	}
	return response{JSONRPC: jsonRPCVersion, ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// --- MCP content / tool-result shapes (mirror server/src/mcp.ts) ---

// content is one block of a tool result: a text block or an image block.
type content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

func textContent(s string) content { return content{Type: "text", Text: s} }
func imageContent(dataB64, mime string) content {
	return content{Type: "image", Data: dataB64, MimeType: mime}
}

// toolResult is the CallToolResult returned from tools/call.
type toolResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

func errorResult(msg string) toolResult {
	return toolResult{Content: []content{textContent("Error: " + msg)}, IsError: true}
}

// toolInfo is a tools/list entry.
type toolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}
