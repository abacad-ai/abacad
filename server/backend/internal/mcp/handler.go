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

// BlobStore mints the signed capability URLs the file-transfer tools hand back to
// the agent (send_file / get_file), so the agent only ever deals in URLs and the
// bytes never cross the MCP surface. Satisfied by an adapter over blob.Service +
// blob.Signer in main.
type BlobStore interface {
	// SignedUploadURL mints a capability URL bound to (accountID, deviceID, path,
	// mode). The agent POSTs bytes to it; the server stores them and delivers them
	// to that device path, returning pass/fail on the POST. Backs send_file.
	SignedUploadURL(accountID, deviceID, path string, mode *int) string
	// SignedDownloadURL mints a capability URL the agent GETs to fetch an existing
	// blob's bytes. Backs get_file.
	SignedDownloadURL(blobID string) string
}

// ResolverFunc builds the per-request DeviceResolver and method Scope from the
// HTTP request (bearer API key -> account + scope). Returning an error rejects the
// request with 401 before any JSON-RPC dispatch.
type ResolverFunc func(r *http.Request) (DeviceResolver, Scope, error)

// Handler serves POST /mcp (Streamable HTTP, stateless).
type Handler struct {
	ResolverFor ResolverFunc
	// Blobs backs send_file / get_file (it mints their signed URLs). May be nil,
	// in which case those tools return a clear "file transfer is not configured"
	// error rather than panic.
	Blobs BlobStore
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

	resp := dispatch(context.WithoutCancel(r.Context()), req, resolver, scope, h.Blobs)
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
