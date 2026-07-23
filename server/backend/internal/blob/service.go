package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"abacad/internal/auth"
	"abacad/internal/store"
)

// ErrTooLarge is returned by Put when the input exceeds the configured cap. The
// HTTP handler maps it (and net/http's own *MaxBytesError) to 413; in-process
// callers (the MCP file-transfer tools) surface it as a tool error.
var ErrTooLarge = errors.New("blob exceeds the maximum size")

// Service is the account-scoped store for data-plane bytes, independent of any
// transport. Both the HTTP /blobs handler and the in-process MCP file-transfer
// tools go through it, so the temp->rename + sha256 discipline lives in exactly
// one place. Bytes stream to and from disk; a multi-GB object never sits in
// memory. See docs/transport.md.
type Service struct {
	Store    *store.Store
	Dir      string // where blob bytes live on disk; must exist
	MaxBytes int64  // reject a single object larger than this
}

// Put streams r to disk (capped, hashed), records its metadata against
// accountID, and returns the stored blob. It never buffers the whole object.
//
// The cap is enforced here too, not only by an HTTP MaxBytesReader: an
// in-process caller passes a plain reader with no network layer to trip. If r's
// own read already failed (e.g. an http.MaxBytesReader upstream), that error is
// returned verbatim so the handler can classify it.
func (s *Service) Put(accountID, contentType string, r io.Reader) (store.Blob, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	tmp, err := os.CreateTemp(s.Dir, "upload-*")
	if err != nil {
		return store.Blob{}, err
	}
	tmpName := tmp.Name()

	// LimitReader to MaxBytes+1 so a copy that reaches MaxBytes+1 bytes tells us
	// the input was over the cap (as opposed to exactly at it). The hash and the
	// write happen in the same pass.
	hasher := sha256.New()
	limited := io.LimitReader(r, s.MaxBytes+1)
	size, copyErr := io.Copy(io.MultiWriter(tmp, hasher), limited)
	closeErr := tmp.Close()

	if copyErr != nil {
		os.Remove(tmpName)
		return store.Blob{}, copyErr
	}
	if closeErr != nil {
		os.Remove(tmpName)
		return store.Blob{}, closeErr
	}
	if size > s.MaxBytes {
		os.Remove(tmpName)
		return store.Blob{}, ErrTooLarge
	}

	id := auth.NewID("blob")
	final := filepath.Join(s.Dir, id)
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return store.Blob{}, err
	}

	b := store.Blob{
		ID:          id,
		AccountID:   accountID,
		ContentType: contentType,
		Size:        size,
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
		CreatedAt:   time.Now().Unix(),
	}
	if err := s.Store.CreateBlob(b); err != nil {
		os.Remove(final)
		return store.Blob{}, err
	}
	return b, nil
}

// Open returns an open file handle to id's bytes plus its metadata, but only if
// the blob belongs to accountID. Missing and cross-account both come back as
// store.ErrNotFound so an id's existence never leaks across accounts. The caller
// closes the returned file.
func (s *Service) Open(accountID, id string) (*os.File, store.Blob, error) {
	b, err := s.Store.BlobByID(id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && b.AccountID != accountID) {
		return nil, store.Blob{}, store.ErrNotFound
	}
	if err != nil {
		return nil, store.Blob{}, err
	}
	f, err := os.Open(filepath.Join(s.Dir, b.ID)) // b.ID is from the DB, not user input
	if err != nil {
		return nil, store.Blob{}, err
	}
	return f, b, nil
}

// OpenByID is Open without the account gate: the caller has already proven
// authorization out of band (a verified capability-URL signature), so ownership
// is not re-checked here. Never call this on an unauthenticated path.
func (s *Service) OpenByID(id string) (*os.File, store.Blob, error) {
	b, err := s.Store.BlobByID(id)
	if err != nil {
		return nil, store.Blob{}, err
	}
	f, err := os.Open(filepath.Join(s.Dir, b.ID)) // b.ID is from the DB, not user input
	if err != nil {
		return nil, store.Blob{}, err
	}
	return f, b, nil
}
