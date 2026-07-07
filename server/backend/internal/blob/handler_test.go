package blob

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"abacad/internal/store"
)

// newFixture spins up a real store + two accounts and a blob handler whose auth
// is driven by an "X-Test-Account" header (A / B), so tests can exercise
// ownership scoping without wiring real tokens.
func newFixture(t *testing.T) (*http.ServeMux, store.Account, store.Account) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	accA, err := st.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account A: %v", err)
	}
	accB, err := st.CreateAccount("b@x.test", "hash")
	if err != nil {
		t.Fatalf("account B: %v", err)
	}

	h := &Handler{
		Store:    st,
		Dir:      t.TempDir(),
		MaxBytes: 1 << 20,
		Account: func(r *http.Request) (store.Account, error) {
			switch r.Header.Get("X-Test-Account") {
			case "A":
				return accA, nil
			case "B":
				return accB, nil
			}
			return store.Account{}, errors.New("unauthorized")
		},
	}
	mux := http.NewServeMux()
	mux.Handle("POST /blobs", http.HandlerFunc(h.Upload))
	mux.Handle("GET /blobs/{id}", http.HandlerFunc(h.Download))
	return mux, accA, accB
}

// upload POSTs body as account and returns the decoded response + status.
func upload(t *testing.T, mux *http.ServeMux, account, contentType string, body []byte) (uploadResponse, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/blobs", bytes.NewReader(body))
	if account != "" {
		req.Header.Set("X-Test-Account", account)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var out uploadResponse
	_ = json.NewDecoder(rr.Body).Decode(&out)
	return out, rr.Code
}

func TestUploadDownloadRoundTrip(t *testing.T) {
	mux, _, _ := newFixture(t)
	body := bytes.Repeat([]byte("abacad-"), 5000) // 35 KB
	wantSum := sha256.Sum256(body)

	res, code := upload(t, mux, "A", "image/jpeg", body)
	if code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201", code)
	}
	if res.Size != int64(len(body)) {
		t.Errorf("size = %d, want %d", res.Size, len(body))
	}
	if res.SHA256 != hex.EncodeToString(wantSum[:]) {
		t.Errorf("sha256 = %s, want %s", res.SHA256, hex.EncodeToString(wantSum[:]))
	}
	if !strings.HasPrefix(res.ID, "blob_") {
		t.Errorf("id = %q, want blob_ prefix", res.ID)
	}

	// Download as the owner.
	req := httptest.NewRequest(http.MethodGet, "/blobs/"+res.ID, nil)
	req.Header.Set("X-Test-Account", "A")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", rr.Code)
	}
	if !bytes.Equal(rr.Body.Bytes(), body) {
		t.Errorf("downloaded bytes differ from uploaded")
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("content-type = %q, want image/jpeg", ct)
	}
}

func TestRangeRequest(t *testing.T) {
	mux, _, _ := newFixture(t)
	body := []byte("0123456789")
	res, _ := upload(t, mux, "A", "application/octet-stream", body)

	req := httptest.NewRequest(http.MethodGet, "/blobs/"+res.ID, nil)
	req.Header.Set("X-Test-Account", "A")
	req.Header.Set("Range", "bytes=2-5")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", rr.Code)
	}
	if got := rr.Body.String(); got != "2345" {
		t.Errorf("range body = %q, want %q", got, "2345")
	}
}

func TestCrossAccountDownloadIs404(t *testing.T) {
	mux, _, _ := newFixture(t)
	res, _ := upload(t, mux, "A", "text/plain", []byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/blobs/"+res.ID, nil)
	req.Header.Set("X-Test-Account", "B") // not the owner
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-account download = %d, want 404 (no existence leak)", rr.Code)
	}
}

func TestUnknownIdIs404(t *testing.T) {
	mux, _, _ := newFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/blobs/blob_nope", nil)
	req.Header.Set("X-Test-Account", "A")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown id = %d, want 404", rr.Code)
	}
}

func TestUnauthorized(t *testing.T) {
	mux, _, _ := newFixture(t)
	// Upload without the account header.
	_, code := upload(t, mux, "", "text/plain", []byte("x"))
	if code != http.StatusUnauthorized {
		t.Errorf("unauth upload = %d, want 401", code)
	}
	// Download without the account header.
	req := httptest.NewRequest(http.MethodGet, "/blobs/blob_x", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("unauth download = %d, want 401", rr.Code)
	}
}

func TestMaxBytesRejected(t *testing.T) {
	mux, _, _ := newFixture(t)
	// The fixture caps at 1 MiB; send 2 MiB.
	big := bytes.Repeat([]byte("x"), 2<<20)
	_, code := upload(t, mux, "A", "application/octet-stream", big)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize upload = %d, want 413", code)
	}
}
