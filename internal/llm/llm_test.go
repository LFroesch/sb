package llm

import (
	"strings"
	"testing"
)

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
