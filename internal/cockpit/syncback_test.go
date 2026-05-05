package cockpit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseItems(t *testing.T) {
	content := `# WORK - demo

## Current Tasks

- first item
- second item
  - nested
- third

## Backlog / Future Features

- later
`
	items := ParseItems(content)
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}
	if items[0].Line != 5 || items[0].Text != "first item" {
		t.Fatalf("bad first item: %+v", items[0])
	}
	if items[2].Indent != 2 {
		t.Fatalf("expected nested indent 2, got %d", items[2].Indent)
	}
}

func TestApplySyncBack_DeletesLinesAndAppendsDevlog(t *testing.T) {
	dir := t.TempDir()
	workPath := filepath.Join(dir, "WORK.md")
	devlogPath := filepath.Join(dir, "DEVLOG.md")

	work := `# WORK - demo

## Current Tasks

- keep me
- delete me
- keep me too
`
	if err := os.WriteFile(workPath, []byte(work), 0o644); err != nil {
		t.Fatal(err)
	}

	devlog := `# DEVLOG

## DevLog

### 2026-04-19 — earlier
- prior entry
`
	if err := os.WriteFile(devlogPath, []byte(devlog), 0o644); err != nil {
		t.Fatal(err)
	}

	job := Job{
		ID:       "j-test",
		PresetID: "claude-senior-dev",
		Sources: []SourceTask{
			{File: workPath, Line: 6, Text: "delete me"},
		},
	}
	touched, err := ApplySyncBack(job, devlogPath)
	if err != nil {
		t.Fatalf("syncback: %v", err)
	}
	if len(touched) != 2 {
		t.Fatalf("expected 2 files touched, got %v", touched)
	}

	got, _ := os.ReadFile(workPath)
	if strings.Contains(string(got), "delete me") {
		t.Fatalf("line not removed:\n%s", got)
	}
	if !strings.Contains(string(got), "keep me") || !strings.Contains(string(got), "keep me too") {
		t.Fatalf("kept lines lost:\n%s", got)
	}

	dl, _ := os.ReadFile(devlogPath)
	if !strings.Contains(string(dl), "delete me") {
		t.Fatalf("devlog missing entry:\n%s", dl)
	}
	if !strings.Contains(string(dl), "prior entry") {
		t.Fatalf("devlog lost earlier entry:\n%s", dl)
	}
}

func TestApplySyncBack_RefusesMismatch(t *testing.T) {
	dir := t.TempDir()
	workPath := filepath.Join(dir, "WORK.md")
	if err := os.WriteFile(workPath, []byte("# W\n\n- totally different\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := Job{Sources: []SourceTask{{File: workPath, Line: 3, Text: "delete me"}}}
	if _, err := ApplySyncBack(job, ""); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestPreviewSyncBack_ShowsWorkAndDevlogChangesWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	workPath := filepath.Join(dir, "WORK.md")
	devlogPath := filepath.Join(dir, "DEVLOG.md")

	work := `# WORK - demo

## Current Tasks

- keep me
- delete me
`
	if err := os.WriteFile(workPath, []byte(work), 0o644); err != nil {
		t.Fatal(err)
	}

	job := Job{
		ID:       "j-test",
		PresetID: "senior-dev",
		Sources: []SourceTask{
			{File: workPath, Line: 6, Text: "delete me"},
		},
	}

	previews, err := PreviewSyncBack(job, devlogPath)
	if err != nil {
		t.Fatalf("PreviewSyncBack: %v", err)
	}
	if len(previews) != 2 {
		t.Fatalf("expected 2 previews, got %d", len(previews))
	}
	if strings.Contains(previews[0].After, "delete me") {
		t.Fatalf("work preview did not remove task:\n%s", previews[0].After)
	}
	if !strings.Contains(previews[1].After, "delete me") {
		t.Fatalf("devlog preview missing task entry:\n%s", previews[1].After)
	}

	afterDisk, err := os.ReadFile(workPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(afterDisk), "delete me") {
		t.Fatalf("preview wrote changes to disk unexpectedly:\n%s", afterDisk)
	}
}
