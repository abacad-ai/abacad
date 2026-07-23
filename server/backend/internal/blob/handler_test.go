package blob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		Svc: &Service{Store: st, Dir: t.TempDir(), MaxBytes: 1 << 20},
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

// newSignedFixture builds a handler with a Signer and a recording Deliver, plus
// the POST /blobs/send and signed GET routes, for the capability-URL paths.
func newSignedFixture(t *testing.T, deliver Deliver) (*http.ServeMux, *Service, *Signer, store.Account) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	acc, err := st.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	svc := &Service{Store: st, Dir: t.TempDir(), MaxBytes: 1 << 20}
	signer := NewSigner([]byte("test-key"), "http://127.0.0.1")
	h := &Handler{
		Svc:     svc,
		Account: func(r *http.Request) (store.Account, error) { return store.Account{}, errors.New("no bearer") },
		Signer:  signer,
		Deliver: deliver,
	}
	mux := http.NewServeMux()
	mux.Handle("GET /blobs/{id}", http.HandlerFunc(h.Download))
	mux.Handle("POST /blobs/send", http.HandlerFunc(h.Send))
	return mux, svc, signer, acc
}

func TestSignedDownloadNeedsNoBearer(t *testing.T) {
	deliver := func(context.Context, string, string, string, *int) (string, error) { return "", nil }
	mux, svc, signer, acc := newSignedFixture(t, deliver)

	body := []byte("hello signed world")
	b, err := svc.Put(acc.ID, "text/plain", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	signed := signer.DownloadURL(b.ID)
	req := httptest.NewRequest(http.MethodGet, signed, nil) // no account header at all
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("signed download = %d, want 200", rr.Code)
	}
	if !bytes.Equal(rr.Body.Bytes(), body) {
		t.Errorf("downloaded bytes differ")
	}

	// Tampered signature → 400.
	u, _ := url.Parse(signed)
	q := u.Query()
	q.Set("sig", "deadbeef")
	u.RawQuery = q.Encode()
	req = httptest.NewRequest(http.MethodGet, u.String(), nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("tampered sig = %d, want 400", rr.Code)
	}
}

func TestSendDeliversAndConfirms(t *testing.T) {
	var gotDevice, gotPath, gotBlob string
	var gotMode *int
	// The Deliver closure records what it was asked to do and echoes back the
	// blob's real sha256 (as a real device would after writing) so the cross-check
	// passes. svc is captured after newSignedFixture returns it.
	var svcRef *Service
	mode := 0o600
	mux, svc, signer, acc := newSignedFixture(t, func(_ context.Context, device, blobID, path string, m *int) (string, error) {
		gotDevice, gotBlob, gotPath, gotMode = device, blobID, path, m
		_, b, err := svcRef.OpenByID(blobID)
		if err != nil {
			return "", err
		}
		return b.SHA256, nil
	})
	svcRef = svc

	body := []byte("payload to write on device")
	up := signer.UploadURL(acc.ID, "dev_1", "/tmp/out.bin", &mode)
	req := httptest.NewRequest(http.MethodPost, up, bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("send = %d, want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var res sendResponse
	if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wantSum := sha256.Sum256(body)
	if !res.Written || res.SHA256 != hex.EncodeToString(wantSum[:]) || res.Path != "/tmp/out.bin" {
		t.Fatalf("unexpected response: %+v", res)
	}
	if gotDevice != "dev_1" || gotPath != "/tmp/out.bin" || gotMode == nil || *gotMode != 0o600 {
		t.Fatalf("deliver got device=%q path=%q mode=%v", gotDevice, gotPath, gotMode)
	}
	if !strings.HasPrefix(gotBlob, "blob_") {
		t.Fatalf("deliver blob id = %q", gotBlob)
	}
	_ = svc
}

func TestSendDeviceOfflineIs504(t *testing.T) {
	mux, _, signer, acc := newSignedFixture(t, func(context.Context, string, string, string, *int) (string, error) {
		return "", ErrDeviceOffline
	})
	up := signer.UploadURL(acc.ID, "dev_1", "/tmp/x", nil)
	req := httptest.NewRequest(http.MethodPost, up, bytes.NewReader([]byte("x")))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("offline send = %d, want 504", rr.Code)
	}
}

func TestSendShaMismatchIs502(t *testing.T) {
	mux, _, signer, acc := newSignedFixture(t, func(context.Context, string, string, string, *int) (string, error) {
		return "0000000000000000000000000000000000000000000000000000000000000000", nil
	})
	up := signer.UploadURL(acc.ID, "dev_1", "/tmp/x", nil)
	req := httptest.NewRequest(http.MethodPost, up, bytes.NewReader([]byte("x")))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("sha mismatch send = %d, want 502", rr.Code)
	}
}

func TestSendBadSignatureIs400(t *testing.T) {
	mux, _, signer, acc := newSignedFixture(t, func(context.Context, string, string, string, *int) (string, error) {
		return "", nil
	})
	up := signer.UploadURL(acc.ID, "dev_1", "/tmp/x", nil)
	u, _ := url.Parse(up)
	q := u.Query()
	q.Set("path", "/tmp/evil") // tamper the destination
	u.RawQuery = q.Encode()
	req := httptest.NewRequest(http.MethodPost, u.String(), bytes.NewReader([]byte("x")))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("tampered upload = %d, want 400", rr.Code)
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
