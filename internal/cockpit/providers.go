package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProviderProfile is a reusable executor definition. Presets carry a
// *suggested* executor, but at launch time any provider can drive any
// preset — classify-with-claude, senior-dev-with-ollama, etc.
//
// Stored one-per-file under <config>/sb/providers/*.json. The file name
// (sans .json) is the fallback ID.
type ProviderProfile struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Executor ExecutorSpec `json:"executor"`
}

// LoadProviders reads every *.json under dir. Invalid files are logged
// and skipped so one bad profile doesn't take down the TUI.
func LoadProviders(dir string) ([]ProviderProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []ProviderProfile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: read provider %s: %v\n", path, err)
			continue
		}
		var p ProviderProfile
		if err := json.Unmarshal(b, &p); err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: parse provider %s: %v\n", path, err)
			continue
		}
		if p.ID == "" {
			p.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		if p.Name == "" {
			p.Name = p.ID
		}
		if isLegacyInteractiveProvider(p.ID) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func isLegacyInteractiveProvider(id string) bool {
	switch id {
	case "claude-interactive", "codex-interactive":
		return true
	default:
		return false
	}
}

// SaveProvider writes a single profile to <dir>/<id>.json.
func SaveProvider(dir string, p ProviderProfile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if p.ID == "" {
		return fmt.Errorf("provider missing id")
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, p.ID+".json"), append(b, '\n'), 0o644)
}

// DeleteProvider removes <dir>/<id>.json. Missing files are treated as
// success so the UI can call it without a pre-check race.
func DeleteProvider(dir, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("provider missing id")
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ProviderPath returns the on-disk path <dir>/<id>.json for a provider.
func ProviderPath(dir, id string) string {
	return filepath.Join(dir, id+".json")
}

// WriteDefaultProviders seeds the providers dir on first run. No-op if
// any provider file already exists.
func WriteDefaultProviders(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			return 0, nil
		}
	}
	seeds := defaultProviders()
	for _, p := range seeds {
		if err := SaveProvider(dir, p); err != nil {
			return 0, err
		}
	}
	return len(seeds), nil
}

// DefaultProviderCount is the number of seed providers written on first run.
var DefaultProviderCount = len(defaultProviders())

func defaultProviders() []ProviderProfile {
	return []ProviderProfile{
		{ID: "claude", Name: "Claude Code", Executor: ExecutorSpec{Type: "claude"}},
		{ID: "codex", Name: "Codex", Executor: ExecutorSpec{Type: "codex"}},
		{ID: "ollama-qwen", Name: "Ollama · qwen2.5:7b", Executor: ExecutorSpec{Type: "ollama", Model: "qwen2.5:7b"}},
		{ID: "ollama-llama", Name: "Ollama · llama3.1:8b", Executor: ExecutorSpec{Type: "ollama", Model: "llama3.1:8b"}},
		{ID: "ollama-gemma", Name: "Ollama · gemma2:9b", Executor: ExecutorSpec{Type: "ollama", Model: "gemma2:9b"}},
	}
}
