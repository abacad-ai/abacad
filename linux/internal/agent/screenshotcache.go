package agent

import (
	"sync"
	"time"

	"abacad-linux/internal/x11"
)

// cacheWindow is how long a captured frame is reused before re-capturing.
const cacheWindow = time.Second

// shotEntry is one cached capture, tagged with the screen generation it belongs
// to so a drive command can invalidate it.
type shotEntry struct {
	shot  x11.Shot
	tree  map[string]any
	stamp time.Time
	gen   int
}

// shotCache is the desktop analogue of the macOS ScreenshotCache: a capture is
// reused for up to cacheWindow, and any drive command bumps the generation so
// the next screenshot is guaranteed fresh (never a frame that predates the
// action). The full single-flight coalescing of the Swift actor is dropped —
// the dispatcher already serializes commands, so at most one capture is ever in
// flight here.
type shotCache struct {
	x    *x11.Conn
	tree func() map[string]any

	mu    sync.Mutex
	gen   int
	entry *shotEntry
}

func newShotCache(x *x11.Conn, tree func() map[string]any) *shotCache {
	return &shotCache{x: x, tree: tree}
}

// invalidate bumps the generation after a screen-changing command.
func (s *shotCache) invalidate() {
	s.mu.Lock()
	s.gen++
	s.mu.Unlock()
}

// screenshot serves the wire result, reusing a fresh-enough cached frame when
// possible. A tree request is not satisfied by a cached frame that lacks one.
func (s *shotCache) screenshot(includeTree bool) (map[string]any, error) {
	s.mu.Lock()
	if e := s.entry; e != nil && e.gen == s.gen &&
		time.Since(e.stamp) < cacheWindow && (!includeTree || e.tree != nil) {
		resp := response(e, includeTree)
		s.mu.Unlock()
		return resp, nil
	}
	curGen := s.gen
	s.mu.Unlock()

	shot, err := s.x.Capture()
	if err != nil {
		return nil, err
	}
	var tree map[string]any
	if includeTree {
		tree = s.tree()
	}
	e := &shotEntry{shot: shot, tree: tree, stamp: time.Now(), gen: curGen}

	s.mu.Lock()
	// Only cache if no drive command bumped the generation mid-capture.
	if e.gen == s.gen {
		s.entry = e
	}
	resp := response(e, includeTree)
	s.mu.Unlock()
	return resp, nil
}

// response builds the screenshot wire dict. The field stays png_base64 for wire
// compatibility even though the bytes are JPEG.
func response(e *shotEntry, includeTree bool) map[string]any {
	r := map[string]any{"w": e.shot.W, "h": e.shot.H, "png_base64": e.shot.Base64}
	if includeTree && e.tree != nil {
		r["tree"] = e.tree
	}
	return r
}
