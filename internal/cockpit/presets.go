package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadPresets reads every *.json file under dir and returns them sorted
// by ID. Invalid files are logged to stderr and skipped so a single bad
// preset doesn't block the TUI.
func LoadPresets(dir string) ([]LaunchPreset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
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
		if p.Hooks.Iteration.Mode == "" {
			p.Hooks.Iteration.Mode = IterationOneShot
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

// SavePreset writes a single preset to <dir>/<id>.json with pretty JSON.
func SavePreset(dir string, p LaunchPreset) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if p.ID == "" {
		return fmt.Errorf("preset missing id")
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, p.ID+".json"), append(b, '\n'), 0o644)
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
			return 0, nil // user already has presets, leave them alone
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

// defaultPresets returns role-centric seeds. Each carries a *suggested*
// provider in its Executor field; at launch time the user can override
// the provider with any ProviderProfile. Shell-flavoured presets
// (test/lint/build/escape) stay provider-locked since the role is the
// shell invocation.
func defaultPresets() []LaunchPreset {
	claude := ExecutorSpec{Type: "claude"}
	codex := ExecutorSpec{Type: "codex"}
	bash := ExecutorSpec{Type: "shell", Cmd: "bash", Args: []string{"-lc"}}

	diffStat := []ShellHook{{Name: "git diff --stat", Cmd: "git diff --stat"}}
	gitStatus := []ShellHook{{Name: "git status", Cmd: "git status --short"}}
	one := IterationPolicy{Mode: IterationOneShot}

	return []LaunchPreset{
		{
			ID: "senior-dev", Name: "Senior dev", Role: "senior-dev",
			SystemPrompt: "You are a senior engineer working in this repo. Make the smallest change that solves the task. " +
				"Before editing, read the relevant files. After editing, run the project's build and tests. " +
				"Commit nothing; the user will review your diff.",
			Executor:    claude,
			Hooks:       HookSpec{PostShell: diffStat, Iteration: one},
			Permissions: "scoped-write",
		},
		{
			ID: "bug-fixer", Name: "Bug fixer", Role: "bug-fixer",
			SystemPrompt: "You fix the specific bug described in the brief. Reproduce first, then fix, then verify. " +
				"Do not refactor surrounding code. If the root cause is outside the reported surface, stop and explain.",
			Executor:    claude,
			Hooks:       HookSpec{PostShell: diffStat, Iteration: one},
			Permissions: "scoped-write",
		},
		{
			ID: "test-writer", Name: "Test writer", Role: "test-writer",
			SystemPrompt: "You add tests that cover the behaviour described in the brief. Match the project's test style. " +
				"Do not modify production code. If a test can't be written without touching prod code, explain why.",
			Executor:    claude,
			Hooks:       HookSpec{PostShell: gitStatus, Iteration: one},
			Permissions: "scoped-write",
		},
		{
			ID: "refactor", Name: "Refactor (narrow)", Role: "refactor",
			SystemPrompt: "You refactor within the bounds the user specifies, without changing behaviour. " +
				"Preserve public APIs unless told otherwise. Keep diffs small and mechanical.",
			Executor:    claude,
			Hooks:       HookSpec{PostShell: diffStat, Iteration: one},
			Permissions: "scoped-write",
		},
		{
			ID: "code-analyzer", Name: "Code analyzer (read-only)", Role: "code-analyzer",
			SystemPrompt: "You analyse the code described in the brief and return a short findings report. " +
				"Do not edit any files. Cover: what the code does, likely bugs, missing tests, risky assumptions.",
			Executor:    claude,
			Hooks:       HookSpec{Iteration: one},
			Permissions: "read-only",
		},
		{
			ID: "explainer", Name: "Explainer", Role: "explainer",
			SystemPrompt: "You explain the code or concept in the brief in plain language. " +
				"Return: one-line summary, the mental model, concrete examples from this repo, common pitfalls.",
			Executor:    claude,
			Hooks:       HookSpec{Iteration: one},
			Permissions: "read-only",
		},
		{
			ID: "pm", Name: "PM (plan mode)", Role: "pm",
			SystemPrompt: "You plan work. Do not write code. Produce: goal, scope cut, ordered task list, risks, " +
				"open questions. Keep the plan tight; prefer cuts over additions.",
			Executor:    claude,
			Hooks:       HookSpec{Iteration: one},
			Permissions: "read-only",
		},
		{
			ID: "docs-writer", Name: "Docs writer", Role: "docs-writer",
			SystemPrompt: "You write or update markdown docs based on current code. Be concrete, cite file paths, " +
				"and avoid marketing language. Update existing files in place rather than creating new ones.",
			Executor:    claude,
			Hooks:       HookSpec{PostShell: gitStatus, Iteration: one},
			Permissions: "scoped-write",
		},
		{
			ID: "scaffold", Name: "Scaffold / generate", Role: "scaffolder",
			SystemPrompt: "You generate new files or scaffolding. Do not modify unrelated files. " +
				"Prefer small, well-named modules and no speculative abstractions.",
			Executor:    codex,
			Hooks:       HookSpec{PostShell: gitStatus, Iteration: one},
			Permissions: "scoped-write",
		},
		{
			ID: "shell-escape", Name: "Shell — escape hatch", Role: "shell",
			Executor:    bash,
			Hooks:       HookSpec{Iteration: one},
			Permissions: "wide-open",
		},
	}
}

func presetSortRank(id string) int {
	switch id {
	case "senior-dev", "bug-fixer", "test-writer", "refactor", "scaffold", "docs-writer":
		return 10
	case "code-analyzer", "explainer", "pm", "rfc":
		return 20
	case "docs-tidy", "classify", "summarize", "shell-test", "shell-lint", "shell-build", "shell-escape":
		return 30
	default:
		return 0
	}
}
