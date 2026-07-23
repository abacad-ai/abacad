package screenshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSweep verifies the retention sweep deletes only screenshots older than the
// TTL and prunes their in-memory timestamps.
func TestSweep(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Save("dev1", []byte{0xff, 0xd8, 0xff}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// A fresh frame is within any positive TTL — nothing removed.
	if n := s.sweep(time.Hour); n != 0 {
		t.Fatalf("fresh sweep removed %d, want 0", n)
	}
	if _, ok := s.At("dev1"); !ok {
		t.Fatal("fresh frame was pruned")
	}

	// Backdate the file two hours; a one-hour TTL now expires it.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "dev1.jpg"), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if n := s.sweep(time.Hour); n != 1 {
		t.Fatalf("stale sweep removed %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "dev1.jpg")); !os.IsNotExist(err) {
		t.Fatalf("stale file still on disk: %v", err)
	}
	if _, ok := s.At("dev1"); ok {
		t.Fatal("stale frame's timestamp not pruned")
	}
}
