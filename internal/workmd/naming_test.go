package workmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LFroesch/sb/internal/config"
)

func TestDiscoverUsesTitleFirstThenCollisionFallback(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "work")
	mustMkdirAll(t, filepath.Join(root, "alpha"))
	mustMkdirAll(t, filepath.Join(root, "beta"))
	mustWriteFile(t, filepath.Join(root, "alpha", "WORK.md"), "# WORK - app\nalpha app\n")
	mustWriteFile(t, filepath.Join(root, "beta", "WORK.md"), "# WORK - app\nbeta app\n")

	projects := Discover([]config.ScanRoot{{Name: "work", Path: root}}, []string{"WORK.md"}, nil, nil, &config.Config{LabelMaxDepth: 2})
	names := projectNames(projects)
	assertContains(t, names, "app (alpha)")
	assertContains(t, names, "app (beta)")
}

func TestDiscoverExpandsFallbackPathsAndUsesRootWhenNeeded(t *testing.T) {
	tmp := t.TempDir()
	workRoot := filepath.Join(tmp, "work")
	clientRoot := filepath.Join(tmp, "client")
	mustMkdirAll(t, filepath.Join(workRoot, "api"))
	mustMkdirAll(t, filepath.Join(clientRoot, "api"))
	mustWriteFile(t, filepath.Join(workRoot, "api", "WORK.md"), "# WORK - work-api\nwork api\n")
	mustWriteFile(t, filepath.Join(clientRoot, "api", "WORK.md"), "# WORK - client-api\nclient api\n")

	projects := Discover([]config.ScanRoot{
		{Name: "work", Path: workRoot},
		{Name: "client", Path: clientRoot},
	}, []string{"WORK.md"}, nil, nil, &config.Config{LabelMaxDepth: 2})
	names := projectNames(projects)
	assertContains(t, names, "work-api")
	assertContains(t, names, "client-api")
}

func TestDiscoverUsesNonWorkTitleLabel(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "plans")
	mustMkdirAll(t, filepath.Join(root, "toolkit"))
	mustWriteFile(t, filepath.Join(root, "toolkit", "ROADMAP.md"), "# ROADMAP - toolkit\nv1 polish\n")

	projects := Discover([]config.ScanRoot{{Name: "plans", Path: root}}, []string{"ROADMAP.md"}, nil, nil, &config.Config{LabelMaxDepth: 2})
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "toolkit" {
		t.Fatalf("expected toolkit, got %q", projects[0].Name)
	}
	if projects[0].Description != "v1 polish" {
		t.Fatalf("expected description, got %q", projects[0].Description)
	}
}

func TestDiscoverExtractsPhaseAndPreview(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "work")
	mustMkdirAll(t, filepath.Join(root, "alpha"))
	mustWriteFile(t, filepath.Join(root, "alpha", "WORK.md"), `# WORK - app
alpha app

## Current Phase

shipping metadata refresh

## Current Tasks

- first task
- second task

## Backlog / Future Features

- later
`)

	projects := Discover([]config.ScanRoot{{Name: "work", Path: root}}, []string{"WORK.md"}, nil, nil, &config.Config{LabelMaxDepth: 2})
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Phase != "shipping metadata refresh" {
		t.Fatalf("phase = %q", projects[0].Phase)
	}
	if len(projects[0].ActivePreview) != 2 {
		t.Fatalf("preview len = %d, want 2", len(projects[0].ActivePreview))
	}
	if projects[0].ActivePreview[0] != "first task" || projects[0].ActivePreview[1] != "second task" {
		t.Fatalf("preview = %#v", projects[0].ActivePreview)
	}
}

func TestDiscoverSkipsBlacklistedScanRootPaths(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "work")
	mustMkdirAll(t, filepath.Join(root, "keep"))
	mustMkdirAll(t, filepath.Join(root, "archive"))
	mustWriteFile(t, filepath.Join(root, "keep", "WORK.md"), "# WORK - keep\nactive project\n")
	mustWriteFile(t, filepath.Join(root, "archive", "WORK.md"), "# WORK - archive\nold project\n")

	projects := Discover([]config.ScanRoot{{Name: "work", Path: root}}, []string{"WORK.md"}, nil, nil, &config.Config{
		LabelMaxDepth:     2,
		ScanBlacklistDirs: []string{"archive"},
	})
	names := projectNames(projects)
	assertContains(t, names, "keep")
	assertNotContains(t, names, "archive")
}

func TestDiscoverSkipsBlacklistedIdeaDirFiles(t *testing.T) {
	tmp := t.TempDir()
	scanRoot := filepath.Join(tmp, "scan")
	ideas := filepath.Join(tmp, "ideas")
	mustMkdirAll(t, scanRoot)
	mustMkdirAll(t, ideas)
	mustWriteFile(t, filepath.Join(ideas, "keep.md"), "# NOTE - keep\nship this\n")
	mustWriteFile(t, filepath.Join(ideas, "ignore.md"), "# NOTE - ignore\nskip this\n")

	projects := Discover([]config.ScanRoot{{Name: "scan", Path: scanRoot}}, nil, nil, []string{ideas}, &config.Config{
		LabelMaxDepth:         2,
		ScanBlacklistNames:    []string{"ignore.md"},
		ScanBlacklistSuffixes: []string{".bak"},
	})
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "keep" {
		t.Fatalf("expected keep, got %q", projects[0].Name)
	}
}

func TestDiscoverIncludesExplicitPathsOutsideFilePatterns(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "work")
	mustMkdirAll(t, filepath.Join(root, "runx"))
	explicit := filepath.Join(root, "runx", "script_ideas.md")
	mustWriteFile(t, explicit, "# NOTE - script_ideas\nscript backlog\n")

	projects := Discover([]config.ScanRoot{{Name: "work", Path: root}}, []string{"WORK.md"}, []string{explicit}, nil, &config.Config{LabelMaxDepth: 2})
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].RelativePath != "script_ideas" {
		t.Fatalf("relative path = %q, want script_ideas", projects[0].RelativePath)
	}
}

func projectNames(projects []Project) []string {
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		names = append(names, p.Name)
	}
	return names
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, v := range values {
		if v == want {
			return
		}
	}
	t.Fatalf("expected %q in %v", want, values)
}

func assertNotContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, v := range values {
		if v == want {
			t.Fatalf("did not expect %q in %v", want, values)
		}
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
