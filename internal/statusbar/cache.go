package statusbar

import (
	"os"
	"path/filepath"
	"time"
)

// cacheDir resolves <state>/sb/cache, creating it on demand. Follows
// the same XDG convention as cockpit.Paths but kept local to avoid an
// import cycle.
func cacheDir() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		if home, err := os.UserHomeDir(); err == nil {
			state = filepath.Join(home, ".local", "state")
		}
	}
	dir := filepath.Join(state, "sb", "cache")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func cacheFile(name string) string { return filepath.Join(cacheDir(), name) }

// readFreshCache returns the cached bytes only if the file's mtime is
// within ttl. Stale or missing → (nil, false).
func readFreshCache(path string, ttl time.Duration) ([]byte, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(st.ModTime()) > ttl {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return b, true
}

func writeCache(path string, body []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
