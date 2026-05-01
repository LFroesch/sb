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

func TestReconcileMissingSectionsReinjectsNonCanonicalBlocks(t *testing.T) {
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
	if !strings.Contains(got, "## DevLog") {
		t.Fatalf("missing non-canonical section: %q", got)
	}
	if !strings.Contains(got, "### 2026-04-27") {
		t.Fatalf("missing section body: %q", got)
	}
}

func TestProjectNameFromContentSupportsTypedTitles(t *testing.T) {
	if got := projectNameFromContent("# ROADMAP - toolkit\nsummary\n"); got != "toolkit" {
		t.Fatalf("got %q", got)
	}
	if got := projectNameFromContent("# WORK - demo | old style\n"); got != "demo" {
		t.Fatalf("got %q", got)
	}
}
