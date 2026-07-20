package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadDotenv reads a .env file into the process environment before flags are
// defined, so envOr() picks the values up. Variables already set in the real
// environment always win — the file is a fallback for local dev, never an
// override of what compose or the shell passed in.
//
// The file is $ABACAD_ENV_FILE when set, otherwise the nearest .env found
// walking up from the working directory (so `cd server/backend && go run …`
// picks up the repo-root .env). A missing file is not an error.
func loadDotenv() {
	path := os.Getenv("ABACAD_ENV_FILE")
	if path == "" {
		path = findDotenv()
	}
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		key, val, ok := parseDotenvLine(s.Text())
		if !ok {
			continue
		}
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, val)
		}
	}
}

// findDotenv returns the nearest .env at or above the working directory.
func findDotenv() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if p := filepath.Join(dir, ".env"); fileExists(p) {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir { // filesystem root
			return ""
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// parseDotenvLine splits one KEY=VALUE line. It tolerates a leading "export ",
// blank lines, # comments, and single- or double-quoted values.
func parseDotenvLine(line string) (key, val string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, val, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if key == "" {
		return "", "", false
	}
	if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
		val = val[1 : len(val)-1]
	}
	return key, val, true
}
