package replay

import (
	"os"
	"path/filepath"
	"time"
)

// Frames holds recorded frames on disk, one JPEG per frame id. It is the same
// bytes-in/bytes-out shape as screenshot.Store, with one difference that drives
// everything else: screenshot.Store keys by DEVICE and overwrites, so it holds
// exactly one file per device forever; this keys by FRAME and only ever appends,
// so its size is bounded by retention and the per-device step cap rather than by
// the number of devices. All methods are safe for concurrent use.
type Frames struct{ dir string }

// OpenFrames prepares the frame directory, creating it if needed. Unlike
// screenshot.Open there is nothing to index on startup: a frame's existence is
// recorded in the replay_steps table, which survives a restart on its own.
func OpenFrames(dir string) (*Frames, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Frames{dir: dir}, nil
}

// Dir is where frames are stored.
func (f *Frames) Dir() string { return f.dir }

// path maps a frame id to its file. Ids are minted by auth.NewID and are safe
// path segments, but one arrives from a URL on the download route, so Base
// strips any separator before it reaches the filesystem.
func (f *Frames) path(frameID string) string {
	return filepath.Join(f.dir, filepath.Base(frameID)+".jpg")
}

// Save writes one frame. Temp file + rename, so a reader never sees a partial
// image — the same discipline screenshot.Store uses.
func (f *Frames) Save(frameID string, jpeg []byte) error {
	tmp, err := os.CreateTemp(f.dir, "frame-*")
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
	if err := os.Rename(tmpName, f.path(frameID)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Open returns an open reader for one frame plus its mod time, for caching
// headers. The caller closes the file.
func (f *Frames) Open(frameID string) (*os.File, time.Time, error) {
	file, err := os.Open(f.path(frameID))
	if err != nil {
		return nil, time.Time{}, err
	}
	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, time.Time{}, err
	}
	return file, fi.ModTime(), nil
}

// Delete removes frames by id. A missing file is not an error: the row is
// already gone by the time this is called, and a delete that has to be retried
// must be able to run to completion over a partial previous attempt.
func (f *Frames) Delete(frameIDs ...string) {
	for _, id := range frameIDs {
		_ = os.Remove(f.path(id))
	}
}

// sweepOrphans deletes frame files older than ttl regardless of what the
// database says. Rows are deleted before their files (see store/replay.go), so a
// crash in that window strands bytes with no row left to find them by — and a
// stranded screen capture is exactly what the retention window promises to
// remove. Anything older than the window is unreachable by definition: every
// live frame's row would itself have been pruned by then. Returns the count, for
// tests.
func (f *Frames) sweepOrphans(ttl time.Duration) int {
	if ttl <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-ttl)
	entries, _ := os.ReadDir(f.dir)
	removed := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jpg" {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(f.dir, e.Name())) == nil {
			removed++
		}
	}
	return removed
}
