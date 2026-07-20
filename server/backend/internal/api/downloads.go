package api

import (
	"net/http"
	"os"
	"regexp"
	"sort"
)

// The public downloads page lists the client builds that actually exist on the
// deploy volume, rather than a hardcoded list that 404s until someone remembers
// to copy a file. Publishing a build stays a file copy (no restart): drop
// abacad-<platform>-latest.<ext> into the downloads dir and it appears here.

// clientBuild is one published client artifact.
type clientBuild struct {
	Platform  string `json:"platform"` // "macos", "android", … (from the filename)
	File      string `json:"file"`
	URL       string `json:"url"`  // where the browser fetches it (/downloads/<file>)
	Size      int64  `json:"size"` // bytes
	UpdatedAt int64  `json:"updated_at"`
}

// buildName matches the publishing convention. Anything else in the directory
// (checksums, notes, older versioned copies) is not a "latest" build and is
// ignored, so the page only ever offers the current artifact per platform.
var buildName = regexp.MustCompile(`^abacad-([a-z0-9]+)-latest\.[a-z0-9]+$`)

// listDownloads reports the published client builds. Public on purpose — you
// download a client before you have an account, so this must work signed out.
func (a *API) listDownloads(w http.ResponseWriter, _ *http.Request) {
	builds := []clientBuild{}
	entries, err := os.ReadDir(a.DownloadsDir)
	if err != nil {
		// No directory yet (a fresh dev box) simply means nothing is published.
		writeJSON(w, http.StatusOK, builds)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := buildName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		builds = append(builds, clientBuild{
			Platform:  m[1],
			File:      e.Name(),
			URL:       "/downloads/" + e.Name(),
			Size:      fi.Size(),
			UpdatedAt: fi.ModTime().Unix(),
		})
	}
	sort.Slice(builds, func(i, j int) bool { return builds[i].Platform < builds[j].Platform })
	writeJSON(w, http.StatusOK, builds)
}
