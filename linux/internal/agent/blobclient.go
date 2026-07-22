package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// blobClient is the device side of the /blobs data plane: the file-transfer
// verbs move bytes over HTTP, not the command WebSocket, so a multi-GB file
// never has to be base64'd onto a text frame. It authenticates with the same
// per-device token the socket uses, carried in the Authorization header.
//
// It is streamed end to end: download copies the response body straight to a
// temp file (then renames), upload posts the file handle straight as the request
// body. Neither buffers the whole object in memory.
type blobClient struct {
	base  string // e.g. https://host/blobs
	token string
	hc    *http.Client
}

// newBlobClient derives the /blobs base from the relay's ws(s) URL: the data
// plane lives on the same host, over http(s). A blank base disables transfer
// (the verbs then report it as unconfigured).
func newBlobClient(base, token string) *blobClient {
	return &blobClient{base: base, token: token, hc: &http.Client{}}
}

func (b *blobClient) auth(r *http.Request) {
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
}

// download streams the blob to destPath and returns the bytes written and their
// hex sha256. It writes to a temp file in the destination directory and renames
// into place, so a reader never observes a half-written file. The parent
// directory must already exist.
func (b *blobClient) download(blobID, destPath string, mode os.FileMode) (int64, string, error) {
	req, err := http.NewRequest(http.MethodGet, b.base+"/"+url.PathEscape(blobID), nil)
	if err != nil {
		return 0, "", err
	}
	b.auth(req)
	resp, err := b.hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("blob download failed: %s%s", resp.Status, snippet(resp.Body))
	}

	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, ".abacad-dl-*")
	if err != nil {
		return 0, "", err
	}
	tmpName := tmp.Name()

	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil {
		os.Remove(tmpName)
		return 0, "", copyErr
	}
	if closeErr != nil {
		os.Remove(tmpName)
		return 0, "", closeErr
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return 0, "", err
	}
	if err := os.Rename(tmpName, destPath); err != nil {
		os.Remove(tmpName)
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// upload streams srcPath to /blobs and returns the new blob id, size, and sha256.
func (b *blobClient) upload(srcPath string) (id string, size int64, sha string, err error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", 0, "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", 0, "", err
	}
	if fi.IsDir() {
		return "", 0, "", fmt.Errorf("%s is a directory, not a file", srcPath)
	}

	req, err := http.NewRequest(http.MethodPost, b.base, f)
	if err != nil {
		return "", 0, "", err
	}
	req.ContentLength = fi.Size() // let the server stream rather than chunk
	req.Header.Set("Content-Type", "application/octet-stream")
	b.auth(req)

	resp, err := b.hc.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", 0, "", fmt.Errorf("blob upload failed: %s%s", resp.Status, snippet(resp.Body))
	}
	var out struct {
		ID     string `json:"id"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, "", fmt.Errorf("bad blob upload response: %w", err)
	}
	return out.ID, out.Size, out.SHA256, nil
}

// snippet reads a short prefix of an error body to attach to the message, so a
// failed transfer says why (e.g. "blob not found") rather than just a status.
func snippet(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 200))
	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}
	return " — " + s
}
