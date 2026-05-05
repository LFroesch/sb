package workmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LFroesch/sb/internal/config"
)

func TestAuditDiscoveredFilesIgnoresIdeaDirsAndExplicitPaths(t *testing.T) {
	root := t.TempDir()

	taskDir := filepath.Join(root, "projects", "alpha")
	ideaDir := filepath.Join(root, "ideas")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ideaDir, 0o755); err != nil {
		t.Fatal(err)
	}

	taskPath := filepath.Join(taskDir, "WORK.md")
	ideaPath := filepath.Join(ideaDir, "concept.md")
	explicitPath := filepath.Join(root, "script_ideas.md")

	if err := os.WriteFile(taskPath, []byte("# WORK - alpha\n\n## Current Tasks\n- task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ideaPath, []byte("# concept\n\n## Notes\n- rough idea\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(explicitPath, []byte("# script ideas\n\n## Scratch\n- keep this loose\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		ScanRoots: []config.ScanRoot{{Name: "projects", Path: filepath.Join(root, "projects")}},
		FilePatterns: []string{"WORK.md"},
		IdeaDirs: []string{ideaDir},
		ExplicitPaths: []string{explicitPath},
	}

	issues, err := AuditDiscoveredFiles(cfg)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected exactly one audited file, got %d: %#v", len(issues), issues)
	}
	if issues[0].Path != taskPath {
		t.Fatalf("expected task file to be audited, got %s", issues[0].Path)
	}
}
