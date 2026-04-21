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
	mustWriteFile(t, filepath.Join(root, "alpha", "WORK.md"), "# WORK - app | alpha app\n")
	mustWriteFile(t, filepath.Join(root, "beta", "WORK.md"), "# WORK - app | beta app\n")

	projects := Discover([]config.ScanRoot{{Name: "work", Path: root}}, []string{"WORK.md"}, nil, &config.Config{LabelMaxDepth: 2})
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
	mustWriteFile(t, filepath.Join(workRoot, "api", "WORK.md"), "# WORK\n")
	mustWriteFile(t, filepath.Join(clientRoot, "api", "WORK.md"), "# WORK\n")

	projects := Discover([]config.ScanRoot{
		{Name: "work", Path: workRoot},
		{Name: "client", Path: clientRoot},
	}, []string{"WORK.md"}, nil, &config.Config{LabelMaxDepth: 2})
	names := projectNames(projects)
	assertContains(t, names, "work/api")
	assertContains(t, names, "client/api")
}

func TestDiscoverUsesNonWorkTitleLabel(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "plans")
	mustMkdirAll(t, filepath.Join(root, "toolkit"))
	mustWriteFile(t, filepath.Join(root, "toolkit", "ROADMAP.md"), "# ROADMAP - toolkit | v1 polish\n")

	projects := Discover([]config.ScanRoot{{Name: "plans", Path: root}}, []string{"ROADMAP.md"}, nil, &config.Config{LabelMaxDepth: 2})
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
