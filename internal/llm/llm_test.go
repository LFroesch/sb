package llm

import (
	"strings"
	"testing"
)

func TestRenderProjectListIncludesBoundedContext(t *testing.T) {
	got := renderProjectList([]ProjectDesc{{
		Name:        "sb",
		Description: strings.Repeat("desc ", 50),
		Phase:       "shipping metadata split",
		Preview:     []string{"first task", "second task", "third task"},
	}})

	if !strings.Contains(got, "- sb — ") {
		t.Fatalf("missing project line: %q", got)
	}
	if strings.Count(got, "active: ") != 2 {
		t.Fatalf("expected 2 preview lines, got %q", got)
	}
	if strings.Contains(got, "third task") {
		t.Fatalf("expected preview truncation, got %q", got)
	}
}

func TestReconcileMissingSectionsKeepsCanonicalOutputOnly(t *testing.T) {
	original := `# WORK - demo

## Current Tasks

- keep this

## DevLog

### 2026-04-27
- shipped thing
`
	cleaned := `# WORK - demo

## Current Tasks

- keep this
`

	got := reconcileMissingSections(original, cleaned)
	if strings.Contains(got, "## DevLog") {
		t.Fatalf("unexpected non-canonical section: %q", got)
	}
}

func TestProjectNameFromContentSupportsTypedTitles(t *testing.T) {
	if got := projectNameFromContent("# ROADMAP - toolkit\nsummary\n"); got != "toolkit" {
		t.Fatalf("got %q", got)
	}
	if got := projectNameFromContent("# WORK - demo\nsummary\n"); got != "demo" {
		t.Fatalf("got %q", got)
	}
}

func TestStripMarkdownFenceHandlesCommonCodeFences(t *testing.T) {
	raw := "```json\n[{\"text\":\"x\"}]\n```"
	if got := stripMarkdownFence(raw); got != "[{\"text\":\"x\"}]" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSONArrayFindsArrayInsideChatter(t *testing.T) {
	raw := "Here you go:\n```json\n[{\"text\":\"x\",\"project\":\"demo\",\"section\":\"current_tasks\"}]\n```"
	got, err := extractJSONArray(raw)
	if err != nil {
		t.Fatalf("extractJSONArray: %v", err)
	}
	if !strings.HasPrefix(got, "[{") {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSONObjectFindsObjectInsideChatter(t *testing.T) {
	raw := "Result:\n{\"text\":\"x\",\"project\":\"demo\",\"section\":\"current_tasks\"}\nThanks"
	got, err := extractJSONObject(raw)
	if err != nil {
		t.Fatalf("extractJSONObject: %v", err)
	}
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
		t.Fatalf("got %q", got)
	}
}
