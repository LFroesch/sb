package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadHookBundles reads every *.json under dir. Invalid files are
// logged and skipped so one bad bundle doesn't take down the TUI.
func LoadHookBundles(dir string) ([]HookBundle, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []HookBundle
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: read hook bundle %s: %v\n", path, err)
			continue
		}
		var h HookBundle
		if err := json.Unmarshal(b, &h); err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: parse hook bundle %s: %v\n", path, err)
			continue
		}
		if h.ID == "" {
			h.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		if h.Name == "" {
			h.Name = h.ID
		}
		if h.Iteration.Mode == "" {
			h.Iteration.Mode = IterationOneShot
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// SaveHookBundle writes a single bundle to <dir>/<id>.json.
func SaveHookBundle(dir string, h HookBundle) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if h.ID == "" {
		return fmt.Errorf("hook bundle missing id")
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, h.ID+".json"), append(b, '\n'), 0o644)
}

// DeleteHookBundle removes <dir>/<id>.json. Missing files are treated
// as success so the UI can call it without a pre-check race.
func DeleteHookBundle(dir, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("hook bundle missing id")
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HookBundlePath returns the on-disk path <dir>/<id>.json.
func HookBundlePath(dir, id string) string {
	return filepath.Join(dir, id+".json")
}

// WriteDefaultHookBundles seeds the hooks dir on first run. No-op if
// any bundle file already exists.
func WriteDefaultHookBundles(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			return 0, nil
		}
	}
	seeds := defaultHookBundles()
	for _, h := range seeds {
		if err := SaveHookBundle(dir, h); err != nil {
			return 0, err
		}
	}
	return len(seeds), nil
}

// DefaultHookBundleCount is the number of seed bundles written on first run.
var DefaultHookBundleCount = len(defaultHookBundles())

func defaultHookBundles() []HookBundle {
	one := IterationPolicy{Mode: IterationOneShot}
	return []HookBundle{
		{
			ID: "diff-stat", Name: "Post: git diff --stat",
			PostShell: []ShellHook{{Name: "git diff --stat", Cmd: "git diff --stat"}},
			Iteration: one,
		},
		{
			ID: "git-status", Name: "Post: git status --short",
			PostShell: []ShellHook{{Name: "git status", Cmd: "git status --short"}},
			Iteration: one,
		},
		{
			ID: "no-hooks", Name: "No hooks (one-shot)",
			Iteration: one,
		},
	}
}
