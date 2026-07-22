// Package blob serves the /blobs data plane: a generic, type-agnostic store for
// binary payloads (files, screenshots, media) that must NOT ride the device
// WebSocket. Callers upload bytes and get back an opaque id; control frames pass
// that id around; anyone authorized for the owning account fetches the bytes back
// over HTTP. The bytes are streamed straight to and from disk, so a multi-GB
// object never sits in memory. See docs/transport.md.
//
// This is deliberately dumb: it never branches on what a blob "is". A screenshot
// and a 1 GB file take the same path. Meaning lives in the control frames that
// reference the id, not here. The storage discipline lives in Service; this file
// is only the HTTP skin over it.
package blob

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"abacad/internal/store"
)

// AccountResolver authenticates a request to an account. It accepts any of the
// server's identities (dashboard session, MCP bearer, device token) — whichever
// the caller presents — and returns the owning account, or an error to reject
// with 401. Built in main so this package stays unaware of how auth is wired.
type AccountResolver func(r *http.Request) (store.Account, error)

// Handler serves POST /blobs and GET /blobs/{id} over a Service.
type Handler struct {
	Svc     *Service
	Account AccountResolver
}

type uploadResponse struct {
	ID     string `json:"id"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Upload handles POST /blobs: stream the request body to disk (capped, hashed),
// record its metadata against the caller's account, and return the id.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	acc, err := h.Account(r)
	if err != nil {
		unauthorized(w, err)
		return
	}
	// Cap the body at the network edge too: reads past MaxBytes fail with
	// *http.MaxBytesError instead of letting a client fill the disk. Service.Put
	// enforces the same cap for in-process callers that have no such reader.
	r.Body = http.MaxBytesReader(w, r.Body, h.Svc.MaxBytes)

	b, err := h.Svc.Put(acc.ID, r.Header.Get("Content-Type"), r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		switch {
		case errors.As(err, &mbe), errors.Is(err, ErrTooLarge):
			writeErr(w, http.StatusRequestEntityTooLarge, "blob exceeds the maximum upload size")
		default:
			writeErr(w, http.StatusInternalServerError, "could not store blob")
		}
		return
	}

	writeJSON(w, http.StatusCreated, uploadResponse{ID: b.ID, Size: b.Size, SHA256: b.SHA256})
}

// Download handles GET /blobs/{id}: stream the bytes back to an authorized
// caller. Range requests (resume) come free via http.ServeContent.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	acc, err := h.Account(r)
	if err != nil {
		unauthorized(w, err)
		return
	}
	f, b, err := h.Svc.Open(acc.ID, r.PathValue("id"))
	// 404 whether it doesn't exist or isn't yours — never leak another account's
	// blob ids by distinguishing "not found" from "forbidden".
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "blob not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load blob")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", b.ContentType)
	// Ids are unique and never rewritten, so the content is immutable.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", time.Unix(b.CreatedAt, 0), f)
}

func unauthorized(w http.ResponseWriter, err error) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="abacad"`)
	writeErr(w, http.StatusUnauthorized, err.Error())
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
