package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadPrompts reads every *.json under dir. Invalid files are logged
// and skipped so one bad file doesn't take down the TUI.
func LoadPrompts(dir string) ([]PromptTemplate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []PromptTemplate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: read prompt %s: %v\n", path, err)
			continue
		}
		var p PromptTemplate
		if err := json.Unmarshal(b, &p); err != nil {
			fmt.Fprintf(os.Stderr, "cockpit: parse prompt %s: %v\n", path, err)
			continue
		}
		if p.ID == "" {
			p.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		if p.Name == "" {
			p.Name = p.ID
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// SavePrompt writes a single template to <dir>/<id>.json.
func SavePrompt(dir string, p PromptTemplate) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if p.ID == "" {
		return fmt.Errorf("prompt missing id")
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, p.ID+".json"), append(b, '\n'), 0o644)
}

// DeletePrompt removes <dir>/<id>.json. Missing files are treated as
// success so the UI can call it without a pre-check race.
func DeletePrompt(dir, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("prompt missing id")
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PromptPath returns the on-disk path <dir>/<id>.json.
func PromptPath(dir, id string) string {
	return filepath.Join(dir, id+".json")
}

// WriteDefaultPrompts seeds the prompts dir on first run. No-op if any
// prompt file already exists.
func WriteDefaultPrompts(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			return 0, nil
		}
	}
	seeds := defaultPrompts()
	for _, p := range seeds {
		if err := SavePrompt(dir, p); err != nil {
			return 0, err
		}
	}
	return len(seeds), nil
}

// DefaultPromptCount is the number of seed prompts written on first run.
var DefaultPromptCount = len(defaultPrompts())

func defaultPrompts() []PromptTemplate {
	return []PromptTemplate{
		{
			ID: "senior-dev", Name: "Senior dev",
			Body: "You are a senior engineer working in this repo. Make the smallest change that solves the task. " +
				"Before editing, read the relevant files. After editing, run the project's build and tests. " +
				"Commit nothing; the user will review your diff.",
		},
		{
			ID: "bug-fixer", Name: "Bug fixer",
			Body: "You fix the specific bug described in the brief. Reproduce first, then fix, then verify. " +
				"Do not refactor surrounding code. If the root cause is outside the reported surface, stop and explain.",
		},
		{
			ID: "test-writer", Name: "Test writer",
			Body: "You add tests that cover the behaviour described in the brief. Match the project's test style. " +
				"Do not modify production code. If a test can't be written without touching prod code, explain why.",
		},
		{
			ID: "refactor", Name: "Refactor (narrow)",
			Body: "You refactor within the bounds the user specifies, without changing behaviour. " +
				"Preserve public APIs unless told otherwise. Keep diffs small and mechanical.",
		},
		{
			ID: "code-analyzer", Name: "Code analyzer",
			Body: "You analyse the code described in the brief and return a short findings report. " +
				"Do not edit any files. Cover: what the code does, likely bugs, missing tests, risky assumptions.",
		},
		{
			ID: "explainer", Name: "Explainer",
			Body: "You explain the code or concept in the brief in plain language. " +
				"Return: one-line summary, the mental model, concrete examples from this repo, common pitfalls.",
		},
		{
			ID: "pm", Name: "PM (plan mode)",
			Body: "You plan work. Do not write code. Produce: goal, scope cut, ordered task list, risks, " +
				"open questions. Keep the plan tight; prefer cuts over additions.",
		},
		{
			ID: "docs-writer", Name: "Docs writer",
			Body: "You write or update markdown docs based on current code. Be concrete, cite file paths, " +
				"and avoid marketing language. Update existing files in place rather than creating new ones.",
		},
		{
			ID: "scaffold", Name: "Scaffold / generate",
			Body: "You generate new files or scaffolding. Do not modify unrelated files. " +
				"Prefer small, well-named modules and no speculative abstractions.",
		},
	}
}
