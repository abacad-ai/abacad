package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// maxBody caps the JSON-RPC request body. Tool-call inputs are tiny (image bytes
// flow the other way), so 4 MiB is generous.
const maxBody = 4 << 20

// Scope authorizes which methods the request's API key may call. list_devices is
// always allowed (it only ever returns already-scoped devices); every device
// action tool is checked. Satisfied by store.KeyScope.
type Scope interface {
	AllowsMethod(name string) bool
}

// ResolverFunc builds the per-request DeviceResolver and method Scope from the
// HTTP request (bearer API key -> account + scope). Returning an error rejects the
// request with 401 before any JSON-RPC dispatch.
type ResolverFunc func(r *http.Request) (DeviceResolver, Scope, error)

// Handler serves POST /mcp (Streamable HTTP, stateless).
type Handler struct {
	ResolverFor ResolverFunc
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// Stateless mode: no server-initiated stream, so GET/DELETE are unused.
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method not allowed (stateless MCP: POST only)",
		})
		return
	}

	resolver, scope, err := h.ResolverFor(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="abacad"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		writeRPC(w, errorResponse(nil, codeParseError, "cannot read body"))
		return
	}
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPC(w, errorResponse(nil, codeParseError, "invalid JSON"))
		return
	}
	if req.JSONRPC != jsonRPCVersion {
		writeRPC(w, errorResponse(req.ID, codeInvalidRequest, "jsonrpc must be \"2.0\""))
		return
	}

	resp := dispatch(context.WithoutCancel(r.Context()), req, resolver, scope)
	if resp == nil {
		// Notification (e.g. notifications/initialized): acknowledge, no body.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRPC(w, *resp)
}

func writeRPC(w http.ResponseWriter, resp response) {
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
