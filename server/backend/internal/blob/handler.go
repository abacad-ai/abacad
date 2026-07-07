// Package blob serves the /blobs data plane: a generic, type-agnostic store for
// binary payloads (files, screenshots, media) that must NOT ride the device
// WebSocket. Callers upload bytes and get back an opaque id; control frames pass
// that id around; anyone authorized for the owning account fetches the bytes back
// over HTTP. The bytes are streamed straight to and from disk, so a multi-GB
// object never sits in memory. See docs/transport.md.
//
// This is deliberately dumb: it never branches on what a blob "is". A screenshot
// and a 1 GB file take the same path. Meaning lives in the control frames that
// reference the id, not here.
package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"abacad/internal/auth"
	"abacad/internal/store"
)

// AccountResolver authenticates a request to an account. It accepts any of the
// server's identities (dashboard session, MCP bearer, device token) — whichever
// the caller presents — and returns the owning account, or an error to reject
// with 401. Built in main so this package stays unaware of how auth is wired.
type AccountResolver func(r *http.Request) (store.Account, error)

// Handler serves POST /blobs and GET /blobs/{id}.
type Handler struct {
	Store    *store.Store
	Dir      string // where blob bytes live on disk; must exist
	MaxBytes int64  // reject a single upload larger than this
	Account  AccountResolver
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
	// Cap the body: reads past MaxBytes fail with *http.MaxBytesError instead of
	// letting a client fill the disk.
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxBytes)

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	tmp, err := os.CreateTemp(h.Dir, "upload-*")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not stage upload")
		return
	}
	tmpName := tmp.Name()

	// Stream to disk and hash in one pass — the bytes never accumulate in memory.
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hasher), r.Body)
	closeErr := tmp.Close()

	if copyErr != nil {
		os.Remove(tmpName)
		var mbe *http.MaxBytesError
		if errors.As(copyErr, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "blob exceeds the maximum upload size")
			return
		}
		writeErr(w, http.StatusBadRequest, "upload read failed")
		return
	}
	if closeErr != nil {
		os.Remove(tmpName)
		writeErr(w, http.StatusInternalServerError, "could not finalize upload")
		return
	}

	id := auth.NewID("blob")
	final := filepath.Join(h.Dir, id)
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		writeErr(w, http.StatusInternalServerError, "could not store blob")
		return
	}

	b := store.Blob{
		ID:          id,
		AccountID:   acc.ID,
		ContentType: contentType,
		Size:        size,
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
		CreatedAt:   time.Now().Unix(),
	}
	if err := h.Store.CreateBlob(b); err != nil {
		os.Remove(final)
		writeErr(w, http.StatusInternalServerError, "could not record blob")
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
	b, err := h.Store.BlobByID(r.PathValue("id"))
	// 404 whether it doesn't exist or isn't yours — never leak another account's
	// blob ids by distinguishing "not found" from "forbidden".
	if errors.Is(err, store.ErrNotFound) || (err == nil && b.AccountID != acc.ID) {
		writeErr(w, http.StatusNotFound, "blob not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load blob")
		return
	}

	f, err := os.Open(filepath.Join(h.Dir, b.ID)) // b.ID is from the DB, not user input
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "blob bytes missing")
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
