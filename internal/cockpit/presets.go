package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadPresets reads every *.json file under dir, resolves each preset's
// library refs (prompt / hook bundle / engine) into the runtime fields,
// and returns the slice sorted by sort rank then ID. Invalid files and
// presets with unresolvable refs are logged and skipped so a single bad
// preset doesn't block the TUI.
func LoadPresets(dir string, prompts []PromptTemplate, bundles []HookBundle, providers []ProviderProfile) ([]LaunchPreset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	promptByID := map[string]PromptTemplate{}
	for _, p := range prompts {
		promptByID[p.ID] = p
	}
	bundleByID := map[string]HookBundle{}
	for _, b := range bundles {
		bundleByID[b.ID] = b
	}
	providerByID := map[string]ProviderProfile{}
	for _, p := range providers {
		providerByID[p.ID] = p
	}

	var out []LaunchPreset
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: read preset %s: %v\n", path, err)
			continue
		}
		var p LaunchPreset
		if err := json.Unmarshal(b, &p); err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: parse preset %s: %v\n", path, err)
			continue
		}
		if p.ID == "" {
			p.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		if p.Name == "" {
			p.Name = p.ID
		}
		if p.LaunchMode == "" {
			p.LaunchMode = LaunchModeSingleJob
		}
		if err := resolveInto(&p, promptByID, bundleByID, providerByID); err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: resolve preset %s: %v\n", path, err)
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		ri := presetSortRank(out[i].ID)
		rj := presetSortRank(out[j].ID)
		if ri != rj {
			return ri < rj
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ResolvePreset fills the runtime fields (SystemPrompt, Hooks, Executor)
// from the libraries based on the preset's refs. Returns an error if any
// ref is set but missing in the corresponding library. Callers building a
// preset in-memory (e.g. per-run override before LaunchJob) use this to
// flesh out the runtime values before passing into LaunchRequest.
func ResolvePreset(p LaunchPreset, prompts []PromptTemplate, bundles []HookBundle, providers []ProviderProfile) (LaunchPreset, error) {
	promptByID := map[string]PromptTemplate{}
	for _, pt := range prompts {
		promptByID[pt.ID] = pt
	}
	bundleByID := map[string]HookBundle{}
	for _, b := range bundles {
		bundleByID[b.ID] = b
	}
	providerByID := map[string]ProviderProfile{}
	for _, pr := range providers {
		providerByID[pr.ID] = pr
	}
	if err := resolveInto(&p, promptByID, bundleByID, providerByID); err != nil {
		return LaunchPreset{}, err
	}
	return p, nil
}

func resolveInto(p *LaunchPreset, prompts map[string]PromptTemplate, bundles map[string]HookBundle, providers map[string]ProviderProfile) error {
	// Always reset runtime fields so a stale value never sneaks through.
	p.SystemPrompt = ""
	p.Hooks = HookSpec{}
	p.Executor = ExecutorSpec{}

	if p.PromptID != "" {
		pt, ok := prompts[p.PromptID]
		if !ok {
			return fmt.Errorf("prompt %q not found", p.PromptID)
		}
		p.SystemPrompt = pt.Body
	}
	if p.HookBundleID != "" {
		b, ok := bundles[p.HookBundleID]
		if !ok {
			return fmt.Errorf("hook bundle %q not found", p.HookBundleID)
		}
		p.Hooks = HookSpec{
			Prompt:    b.Prompt,
			PreShell:  b.PreShell,
			PostShell: b.PostShell,
			Iteration: b.Iteration,
		}
	}
	if p.EngineID != "" {
		pr, ok := providers[p.EngineID]
		if !ok {
			return fmt.Errorf("engine %q not found", p.EngineID)
		}
		p.Executor = pr.Executor
	}
	if p.Hooks.Iteration.Mode == "" {
		p.Hooks.Iteration.Mode = IterationOneShot
	}
	return nil
}

// SavePreset writes a single preset to <dir>/<id>.json with pretty JSON.
// Only the on-disk fields (identity + refs + permissions/launch_mode) are
// emitted; runtime-resolved fields (SystemPrompt, Executor, Hooks) are
// stripped via a disk-specific struct so an empty struct doesn't leak as
// `"executor": {...}` in the JSON.
func SavePreset(dir string, p LaunchPreset) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if p.ID == "" {
		return fmt.Errorf("preset missing id")
	}
	disk := presetOnDisk{
		ID:           p.ID,
		Name:         p.Name,
		LaunchMode:   p.LaunchMode,
		Permissions:  p.Permissions,
		PromptID:     p.PromptID,
		HookBundleID: p.HookBundleID,
		EngineID:     p.EngineID,
	}
	b, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, p.ID+".json"), append(b, '\n'), 0o644)
}

// presetOnDisk mirrors LaunchPreset's persisted shape. Kept separate so
// the runtime struct can carry resolved fields without leaking them into
// the JSON file.
type presetOnDisk struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	LaunchMode   string `json:"launch_mode,omitempty"`
	Permissions  string `json:"permissions,omitempty"`
	PromptID     string `json:"prompt_id,omitempty"`
	HookBundleID string `json:"hook_bundle_id,omitempty"`
	EngineID     string `json:"engine_id,omitempty"`
}

// DeletePreset removes <dir>/<id>.json. Missing files are treated as
// success so the UI can call it without a pre-check race.
func DeletePreset(dir, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("preset missing id")
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PresetPath returns the on-disk path <dir>/<id>.json for a preset.
func PresetPath(dir, id string) string {
	return filepath.Join(dir, id+".json")
}

// WriteDefaultPresets materialises the seed presets into dir if no
// preset files already exist there. Returns how many it wrote. Users are
// expected to edit or delete seeds; we never overwrite once anything is
// on disk.
func WriteDefaultPresets(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			return 0, nil
		}
	}
	seeds := defaultPresets()
	for _, p := range seeds {
		if err := SavePreset(dir, p); err != nil {
			return 0, err
		}
	}
	return len(seeds), nil
}

// DefaultPresetCount is the number of seed presets written on first run.
// Exposed so tests have one source of truth for the count assertion.
var DefaultPresetCount = len(defaultPresets())

// defaultPresets returns ref-based seeds. Each composes a prompt + a hook
// bundle + an engine from the libraries. The corresponding library
// entries are seeded by defaultPrompts() and defaultHookBundles() (in
// prompts.go and hookbundles.go) plus the existing defaultProviders().
func defaultPresets() []LaunchPreset {
	return []LaunchPreset{
		{
			ID: "senior-dev", Name: "Senior dev",
			PromptID: "senior-dev", HookBundleID: "diff-stat", EngineID: "claude",
			Permissions: "scoped-write",
		},
		{
			ID: "bug-fixer", Name: "Bug fixer",
			PromptID: "bug-fixer", HookBundleID: "diff-stat", EngineID: "claude",
			Permissions: "scoped-write",
		},
		{
			ID: "test-writer", Name: "Test writer",
			PromptID: "test-writer", HookBundleID: "git-status", EngineID: "claude",
			Permissions: "scoped-write",
		},
		{
			ID: "refactor", Name: "Refactor (narrow)",
			PromptID: "refactor", HookBundleID: "diff-stat", EngineID: "claude",
			Permissions: "scoped-write",
		},
		{
			ID: "code-analyzer", Name: "Code analyzer (read-only)",
			PromptID: "code-analyzer", HookBundleID: "no-hooks", EngineID: "claude",
			Permissions: "read-only",
		},
		{
			ID: "explainer", Name: "Explainer",
			PromptID: "explainer", HookBundleID: "no-hooks", EngineID: "claude",
			Permissions: "read-only",
		},
		{
			ID: "pm", Name: "PM (plan mode)",
			PromptID: "pm", HookBundleID: "no-hooks", EngineID: "claude",
			Permissions: "read-only",
		},
		{
			ID: "docs-writer", Name: "Docs writer",
			PromptID: "docs-writer", HookBundleID: "git-status", EngineID: "claude",
			Permissions: "scoped-write",
		},
		{
			ID: "scaffold", Name: "Scaffold / generate",
			PromptID: "scaffold", HookBundleID: "git-status", EngineID: "codex",
			Permissions: "scoped-write",
		},
	}
}

func presetSortRank(id string) int {
	switch id {
	case "senior-dev", "bug-fixer", "test-writer", "refactor", "scaffold", "docs-writer":
		return 10
	case "code-analyzer", "explainer", "pm", "rfc":
		return 20
	case "docs-tidy", "classify", "summarize":
		return 30
	default:
		return 0
	}
}
