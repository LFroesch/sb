package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/workmd"
)

type projectCache struct {
	ConfigKey string           `json:"config_key"`
	SavedAt   time.Time        `json:"saved_at"`
	Projects  []workmd.Project `json:"projects"`
}

func projectCachePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "projects-cache.json"), nil
}

func discoveryConfigKey(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	key := struct {
		ScanRoots               []config.ScanRoot `json:"scan_roots"`
		FilePatterns            []string          `json:"file_patterns"`
		ExplicitPaths           []string          `json:"explicit_paths"`
		IdeaDirs                []string          `json:"idea_dirs"`
		ScanBlacklistNames      []string          `json:"scan_blacklist_names"`
		ScanBlacklistSuffixes   []string          `json:"scan_blacklist_suffixes"`
		ScanBlacklistDirs       []string          `json:"scan_blacklist_dirs"`
		ScanBlacklistSubstrings []string          `json:"scan_blacklist_substrings"`
	}{
		ScanRoots:               cfg.ExpandedScanRoots(),
		FilePatterns:            append([]string(nil), cfg.FilePatterns...),
		ExplicitPaths:           cfg.ExpandedExplicitPaths(),
		IdeaDirs:                cfg.ExpandedIdeaDirs(),
		ScanBlacklistNames:      append([]string(nil), cfg.ScanBlacklistNames...),
		ScanBlacklistSuffixes:   append([]string(nil), cfg.ScanBlacklistSuffixes...),
		ScanBlacklistDirs:       append([]string(nil), cfg.ScanBlacklistDirs...),
		ScanBlacklistSubstrings: append([]string(nil), cfg.ScanBlacklistSubstrings...),
	}
	data, err := json.Marshal(key)
	if err != nil {
		return ""
	}
	return string(data)
}

func loadProjectCache(cfg *config.Config) ([]workmd.Project, bool, error) {
	path, err := projectCachePath()
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var cache projectCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(cache.ConfigKey) == "" || cache.ConfigKey != discoveryConfigKey(cfg) {
		return nil, false, nil
	}
	return cache.Projects, true, nil
}

func saveProjectCache(cfg *config.Config, projects []workmd.Project) error {
	path, err := projectCachePath()
	if err != nil {
		return err
	}
	cache := projectCache{
		ConfigKey: discoveryConfigKey(cfg),
		SavedAt:   time.Now(),
		Projects:  projects,
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func mergeDiscoveredProjects(existing, discovered []workmd.Project) []workmd.Project {
	if len(existing) == 0 || len(discovered) == 0 {
		return discovered
	}
	byPath := make(map[string]workmd.Project, len(existing))
	for _, project := range existing {
		byPath[project.Path] = project
	}
	out := make([]workmd.Project, len(discovered))
	for i, project := range discovered {
		cached, ok := byPath[project.Path]
		if !ok || !cached.Hydrated || cached.ModTime.IsZero() || !cached.ModTime.Equal(project.ModTime) {
			out[i] = project
			continue
		}
		project.Name = cached.Name
		project.Description = cached.Description
		project.Content = cached.Content
		project.Title = cached.Title
		project.Phase = cached.Phase
		project.ActivePreview = cached.ActivePreview
		project.Tasks = cached.Tasks
		project.TaskCount = cached.TaskCount
		project.CurrentCount = cached.CurrentCount
		project.BacklogCount = cached.BacklogCount
		project.NonListCount = cached.NonListCount
		project.Hydrated = true
		out[i] = project
	}
	return out
}
