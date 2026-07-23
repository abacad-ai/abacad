package blob

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"abacad/internal/store"
)

// TestPruneOlderThan verifies the retention sweep deletes a blob's bytes and its
// metadata row exactly when its age crosses the cutoff, and leaves fresher blobs
// intact.
func TestPruneOlderThan(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	acc, err := st.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}

	dir := t.TempDir()
	svc := &Service{Store: st, Dir: dir, MaxBytes: 1 << 20}
	b, err := svc.Put(acc.ID, "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	// Cutoff before the blob's creation — nothing pruned, bytes + row intact.
	if n, err := svc.PruneOlderThan(b.CreatedAt - 1); err != nil || n != 0 {
		t.Fatalf("early cutoff pruned n=%d err=%v, want 0/nil", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, b.ID)); err != nil {
		t.Fatalf("bytes deleted too early: %v", err)
	}
	if _, err := st.BlobByID(b.ID); err != nil {
		t.Fatalf("row deleted too early: %v", err)
	}

	// Cutoff at/after creation — pruned; bytes and row both gone.
	if n, err := svc.PruneOlderThan(b.CreatedAt); err != nil || n != 1 {
		t.Fatalf("cutoff pruned n=%d err=%v, want 1/nil", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, b.ID)); !os.IsNotExist(err) {
		t.Fatalf("bytes still on disk: %v", err)
	}
	if _, err := st.BlobByID(b.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("row still present: %v", err)
	}
}
