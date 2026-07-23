package blob

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func queryOf(rawURL string) url.Values {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u.Query()
}

func TestDownloadURLRoundTrip(t *testing.T) {
	s := NewSigner([]byte("k"), "https://abacad.ai/")
	raw := s.DownloadURL("blob_abc")
	if !strings.HasPrefix(raw, "https://abacad.ai/blobs/blob_abc?") {
		t.Fatalf("unexpected url: %s", raw)
	}
	if err := s.VerifyDownload("blob_abc", queryOf(raw)); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	// Wrong blob id must not verify against this signature.
	if err := s.VerifyDownload("blob_other", queryOf(raw)); err == nil {
		t.Fatal("expected verify to fail for a different blob id")
	}
}

func TestUploadURLRoundTrip(t *testing.T) {
	s := NewSigner([]byte("k"), "https://abacad.ai")
	mode := 0o644
	raw := s.UploadURL("acct_1", "dev_1", "/etc/app.conf", &mode)
	g, err := s.VerifyUpload(queryOf(raw))
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if g.AccountID != "acct_1" || g.DeviceID != "dev_1" || g.Path != "/etc/app.conf" {
		t.Fatalf("grant mismatch: %+v", g)
	}
	if g.Mode == nil || *g.Mode != 0o644 {
		t.Fatalf("mode mismatch: %+v", g.Mode)
	}
}

func TestUploadURLNoMode(t *testing.T) {
	s := NewSigner([]byte("k"), "https://abacad.ai")
	raw := s.UploadURL("acct_1", "dev_1", "/tmp/x", nil)
	g, err := s.VerifyUpload(queryOf(raw))
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if g.Mode != nil {
		t.Fatalf("expected nil mode, got %v", *g.Mode)
	}
}

func TestTamperedParamFailsVerify(t *testing.T) {
	s := NewSigner([]byte("k"), "https://abacad.ai")
	raw := s.UploadURL("acct_1", "dev_1", "/tmp/x", nil)
	q := queryOf(raw)
	q.Set("path", "/tmp/evil") // move the write elsewhere
	if _, err := s.VerifyUpload(q); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
	// Tampering the bound account is likewise rejected.
	q = queryOf(raw)
	q.Set("acct", "acct_2")
	if _, err := s.VerifyUpload(q); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestWrongKeyFailsVerify(t *testing.T) {
	minted := NewSigner([]byte("key-a"), "https://abacad.ai")
	other := NewSigner([]byte("key-b"), "https://abacad.ai")
	raw := minted.DownloadURL("blob_abc")
	if err := other.VerifyDownload("blob_abc", queryOf(raw)); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature under a different key, got %v", err)
	}
}

func TestExpiredURL(t *testing.T) {
	s := NewSigner([]byte("k"), "https://abacad.ai")
	// Hand-craft a validly-signed but already-expired download URL.
	exp := time.Now().Add(-time.Second).Unix()
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", s.sign(downloadCanonical("blob_abc", exp)))
	if err := s.VerifyDownload("blob_abc", q); err != ErrExpired {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestMissingSigAndExp(t *testing.T) {
	s := NewSigner([]byte("k"), "https://abacad.ai")
	if err := s.VerifyDownload("blob_abc", url.Values{}); err != ErrBadSignature {
		t.Fatalf("expected ErrBadSignature for empty query, got %v", err)
	}
}
