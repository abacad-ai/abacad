package blob

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Signed capability URLs let the agent deal only in URLs: get_file mints a
// download URL it GETs, send_file mints an upload URL it POSTs bytes to. A URL
// *is* the authorization — possession plus a valid HMAC signature grants the
// scoped action until it expires. Verification is stateless (pure HMAC + expiry),
// so there is no DB row and nothing to clean up. TTLs are short so a leaked URL
// stops working quickly; the URLs are deliberately NOT single-use, because a file
// write to a fixed path is idempotent and a retry must be able to replay safely.
const (
	downloadTTL = 5 * time.Minute
	uploadTTL   = 5 * time.Minute
)

var (
	// ErrBadSignature covers a missing, malformed, or forged signature. The HTTP
	// layer maps it (and ErrExpired) to 400.
	ErrBadSignature = errors.New("invalid signature")
	// ErrExpired is a validly-signed URL whose exp has passed.
	ErrExpired = errors.New("signature expired")
)

// Signer mints and verifies stateless HMAC capability URLs for the /blobs data
// plane. baseURL is the scheme+host the URLs point at (e.g. https://abacad.ai).
type Signer struct {
	key     []byte
	baseURL string
}

// NewSigner returns a Signer over key, minting URLs under baseURL (trailing slash
// trimmed).
func NewSigner(key []byte, baseURL string) *Signer {
	return &Signer{key: key, baseURL: strings.TrimRight(baseURL, "/")}
}

func (s *Signer) sign(canonical string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(canonical))
	return hex.EncodeToString(m.Sum(nil))
}

func downloadCanonical(blobID string, exp int64) string {
	return fmt.Sprintf("v1\ndownload\n%s\n%d", blobID, exp)
}

func uploadCanonical(acct, device, path, mode string, exp int64) string {
	return fmt.Sprintf("v1\nupload\n%s\n%s\n%s\n%s\n%d", acct, device, path, mode, exp)
}

// DownloadURL returns a signed GET URL for an existing blob, valid for downloadTTL.
func (s *Signer) DownloadURL(blobID string) string {
	exp := time.Now().Add(downloadTTL).Unix()
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", s.sign(downloadCanonical(blobID, exp)))
	return fmt.Sprintf("%s/blobs/%s?%s", s.baseURL, url.PathEscape(blobID), q.Encode())
}

// UploadURL returns a signed POST target bound to (accountID, deviceID, path,
// mode). Posting bytes to it stores them and delivers them to that device path.
// Valid for uploadTTL.
func (s *Signer) UploadURL(accountID, deviceID, path string, mode *int) string {
	exp := time.Now().Add(uploadTTL).Unix()
	modeStr := ""
	if mode != nil {
		modeStr = strconv.Itoa(*mode)
	}
	q := url.Values{}
	q.Set("acct", accountID)
	q.Set("device", deviceID)
	q.Set("path", path)
	if modeStr != "" {
		q.Set("mode", modeStr)
	}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", s.sign(uploadCanonical(accountID, deviceID, path, modeStr, exp)))
	return fmt.Sprintf("%s/blobs/send?%s", s.baseURL, q.Encode())
}

// VerifyDownload checks a download URL's query against blobID.
func (s *Signer) VerifyDownload(blobID string, q url.Values) error {
	exp, err := parseExp(q)
	if err != nil {
		return err
	}
	if err := s.verifySig(downloadCanonical(blobID, exp), q.Get("sig")); err != nil {
		return err
	}
	return checkExpiry(exp)
}

// UploadGrant is what a valid upload URL authorizes: write these bytes to path on
// this account's device.
type UploadGrant struct {
	AccountID string
	DeviceID  string
	Path      string
	Mode      *int
}

// VerifyUpload checks an upload URL's query and returns the grant it carries.
func (s *Signer) VerifyUpload(q url.Values) (UploadGrant, error) {
	exp, err := parseExp(q)
	if err != nil {
		return UploadGrant{}, err
	}
	acct, device, path, modeStr := q.Get("acct"), q.Get("device"), q.Get("path"), q.Get("mode")
	if err := s.verifySig(uploadCanonical(acct, device, path, modeStr, exp), q.Get("sig")); err != nil {
		return UploadGrant{}, err
	}
	if err := checkExpiry(exp); err != nil {
		return UploadGrant{}, err
	}
	g := UploadGrant{AccountID: acct, DeviceID: device, Path: path}
	if modeStr != "" {
		m, err := strconv.Atoi(modeStr)
		if err != nil {
			return UploadGrant{}, ErrBadSignature
		}
		g.Mode = &m
	}
	return g, nil
}

// parseExp parses the exp query param's format only; expiry is checked after the
// signature verifies, so we never act on an unsigned timestamp.
func parseExp(q url.Values) (int64, error) {
	raw := q.Get("exp")
	if raw == "" {
		return 0, ErrBadSignature
	}
	exp, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, ErrBadSignature
	}
	return exp, nil
}

func checkExpiry(exp int64) error {
	if time.Now().Unix() >= exp {
		return ErrExpired
	}
	return nil
}

func (s *Signer) verifySig(canonical, got string) error {
	if got == "" {
		return ErrBadSignature
	}
	want := s.sign(canonical)
	if !hmac.Equal([]byte(want), []byte(got)) {
		return ErrBadSignature
	}
	return nil
}
