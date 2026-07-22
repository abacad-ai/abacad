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

// BlobStore lets the file-transfer tools (push_file / pull_file) stage and read
// data-plane bytes on behalf of the agent's account, so the agent never has to
// leave the MCP surface to move a file. Satisfied by an adapter over
// blob.Service in main. All operations are account-scoped: Open returns the
// not-found error for a blob owned by a different account.
type BlobStore interface {
	// Put stores r's bytes under accountID and returns the new blob's id, size,
	// and hex sha256.
	Put(accountID, contentType string, r io.Reader) (id string, size int64, sha256 string, err error)
	// Open returns a reader over the blob's bytes plus its size and hex sha256,
	// if id belongs to accountID. The caller closes the reader.
	Open(accountID, id string) (rc io.ReadCloser, size int64, sha256 string, err error)
}

// ResolverFunc builds the per-request DeviceResolver and method Scope from the
// HTTP request (bearer API key -> account + scope). Returning an error rejects the
// request with 401 before any JSON-RPC dispatch.
type ResolverFunc func(r *http.Request) (DeviceResolver, Scope, error)

// Handler serves POST /mcp (Streamable HTTP, stateless).
type Handler struct {
	ResolverFor ResolverFunc
	// Blobs backs push_file / pull_file. May be nil, in which case those tools
	// return a clear "file transfer is not configured" error rather than panic.
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
