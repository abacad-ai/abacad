// Package screenshot persists each device's most recent screen capture. The
// server keeps the last JPEG per device on disk (one file per device id) and its
// capture time in memory, so the dashboard can:
//
//   - show a device's screen instantly on load, without waiting for a fresh
//     round-trip to the device, and
//   - keep showing that last frame (grayed) after the device goes offline.
//
// It is deliberately small and dumb: bytes in, bytes out, keyed by device id.
// The only capture path is write-through — the dashboard's live poll saves each
// frame it fetches — so the server never wakes a device on its own.
package screenshot

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Store holds the latest screenshot per device: bytes on disk, timestamps in
// memory. All methods are safe for concurrent use.
type Store struct {
	dir string
	mu  sync.RWMutex
	at  map[string]int64 // deviceID -> unix seconds of the last save
}

// Open prepares the store, creating dir and indexing any screenshots already on
// disk so a restart doesn't lose the last-known frame.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, at: make(map[string]int64)}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jpg" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jpg")
		if info, err := e.Info(); err == nil {
			s.at[id] = info.ModTime().Unix()
		}
	}
	return s, nil
}

// deviceID values come from the store (never user input) and are safe path
// segments, but Base strips any separator just in case.
func (s *Store) path(deviceID string) string {
	return filepath.Join(s.dir, filepath.Base(deviceID)+".jpg")
}

// Save writes jpeg as deviceID's latest screenshot and records the time. The
// replace is atomic (temp file + rename) so a concurrent read never sees a
// half-written frame.
func (s *Store) Save(deviceID string, jpeg []byte) error {
	tmp, err := os.CreateTemp(s.dir, "shot-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(jpeg); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.path(deviceID)); err != nil {
		os.Remove(tmpName)
		return err
	}
	s.mu.Lock()
	s.at[deviceID] = time.Now().Unix()
	s.mu.Unlock()
	return nil
}

// At returns the unix-seconds time of deviceID's last screenshot and whether one
// exists. Used to tell the dashboard a device has a frame to show (and as a
// cache key that changes when the frame does).
func (s *Store) At(deviceID string) (int64, bool) {
	s.mu.RLock()
	at, ok := s.at[deviceID]
	s.mu.RUnlock()
	return at, ok
}

// Open returns an open reader for deviceID's stored screenshot plus its mod time
// (for caching headers). The caller must Close the file. Returns an error if no
// screenshot has been stored.
func (s *Store) Open(deviceID string) (*os.File, time.Time, error) {
	f, err := os.Open(s.path(deviceID))
	if err != nil {
		return nil, time.Time{}, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, time.Time{}, err
	}
	return f, fi.ModTime(), nil
}

// Delete removes a device's stored screenshot (called when the device is
// deleted).
func (s *Store) Delete(deviceID string) {
	_ = os.Remove(s.path(deviceID))
	s.mu.Lock()
	delete(s.at, deviceID)
	s.mu.Unlock()
}
