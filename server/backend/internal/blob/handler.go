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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"abacad/internal/store"
)

// AccountResolver authenticates a request to an account. It accepts any of the
// server's identities (dashboard session, MCP bearer, device token) — whichever
// the caller presents — and returns the owning account, or an error to reject
// with 401. Built in main so this package stays unaware of how auth is wired.
type AccountResolver func(r *http.Request) (store.Account, error)

// ErrDeviceOffline is returned by a Deliver func when the target device has no
// live connection. The Send handler maps it to 504.
var ErrDeviceOffline = errors.New("device is offline")

// Deliver hands a stored blob to a device, telling it to write those bytes to
// destPath (with optional Unix mode), and returns the sha256 the device reports
// after writing. Injected from main so this package stays unaware of the relay.
type Deliver func(ctx context.Context, deviceID, blobID, destPath string, mode *int) (sha256 string, err error)

// Handler serves the /blobs data plane: POST /blobs, GET /blobs/{id}, and
// POST /blobs/send (the signed send_file upload). Signer and Deliver are nil on
// servers with signed transfer unconfigured, which disables the signed paths.
type Handler struct {
	Svc     *Service
	Account AccountResolver
	Signer  *Signer
	Deliver Deliver
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

// Download handles GET /blobs/{id}: stream the bytes back. Two ways to authorize:
// a signed capability URL (?sig=...&exp=..., where the signature IS the grant and
// no bearer is needed — get_file mints these), or the classic bearer path
// (dashboard session / API key / device token). Range requests (resume) come free
// via http.ServeContent.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Signed path: verify the signature, then open without the account gate — the
	// signature already proves authorization for this exact blob.
	if h.Signer != nil && r.URL.Query().Get("sig") != "" {
		if err := h.Signer.VerifyDownload(id, r.URL.Query()); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		f, b, err := h.Svc.OpenByID(id)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "blob not found")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "could not load blob")
			return
		}
		defer f.Close()
		serveBlob(w, r, b, f)
		return
	}

	// Bearer path.
	acc, err := h.Account(r)
	if err != nil {
		unauthorized(w, err)
		return
	}
	f, b, err := h.Svc.Open(acc.ID, id)
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
	serveBlob(w, r, b, f)
}

func serveBlob(w http.ResponseWriter, r *http.Request, b store.Blob, f *os.File) {
	w.Header().Set("Content-Type", b.ContentType)
	// Ids are unique and never rewritten, so the content is immutable.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", time.Unix(b.CreatedAt, 0), f)
}

type sendResponse struct {
	Written bool   `json:"written"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Path    string `json:"path"`
}

// Send handles POST /blobs/send?<signed>: the agent side of send_file. It verifies
// the upload capability, streams the body to a blob, delivers that blob to the
// bound device path, and reports the device-confirmed sha256 — so the agent learns
// pass/fail from this single response. Idempotent: replaying the same POST rewrites
// the same bytes to the same path.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	if h.Signer == nil || h.Deliver == nil {
		writeErr(w, http.StatusNotFound, "signed file transfer is not configured")
		return
	}
	grant, err := h.Signer.VerifyUpload(r.URL.Query())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.Svc.MaxBytes)
	b, err := h.Svc.Put(grant.AccountID, r.Header.Get("Content-Type"), r.Body)
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

	sum, err := h.Deliver(r.Context(), grant.DeviceID, b.ID, grant.Path, grant.Mode)
	if errors.Is(err, ErrDeviceOffline) {
		writeErr(w, http.StatusGatewayTimeout, "device is offline")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, "device write failed: "+err.Error())
		return
	}
	// Cross-check what the device wrote against what we staged.
	if sum != "" && sum != b.SHA256 {
		writeErr(w, http.StatusBadGateway, "integrity check failed: staged "+b.SHA256+" but device wrote "+sum)
		return
	}

	writeJSON(w, http.StatusOK, sendResponse{Written: true, Size: b.Size, SHA256: b.SHA256, Path: grant.Path})
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
