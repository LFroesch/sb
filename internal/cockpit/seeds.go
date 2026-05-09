package cockpit

import (
	"os"
	"path/filepath"
	"strings"
)

func existingSeedIDs(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out[strings.TrimSuffix(filepath.Base(e.Name()), ".json")] = true
	}
	return out, nil
}
