package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config holds sb runtime settings. Load order: defaults → config file → env vars.
type Config struct {
	Model        string   `json:"model"`
	OllamaHost   string   `json:"ollama_host"`
	ScanDirs     []string `json:"scan_dirs"`     // dirs to recursively search for FilePatterns
	FilePatterns []string `json:"file_patterns"` // file names to match (e.g. WORK.md, ROADMAP.md)
	IdeaDirs     []string `json:"idea_dirs"`     // dirs where all .md files are loaded flat
}

const defaultModel = "qwen2.5:7b"
const defaultHost = "http://localhost:11434"

var defaultScanDirs = []string{"~/projects"}
var defaultFilePatterns = []string{"WORK.md"}
var defaultIdeaDirs = []string{"~/projects/active/SECOND_BRAIN/ideas/tui"}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), p[2:])
	}
	return p
}

// ExpandedScanDirs returns ScanDirs with ~ expanded.
func (c *Config) ExpandedScanDirs() []string { return expandAll(c.ScanDirs) }

// ExpandedIdeaDirs returns IdeaDirs with ~ expanded.
func (c *Config) ExpandedIdeaDirs() []string { return expandAll(c.IdeaDirs) }

func expandAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = expandHome(s)
	}
	return out
}

// Load reads ~/.config/sb/config.json (if present), then applies env var overrides.
func Load() *Config {
	cfg := &Config{
		Model:        defaultModel,
		OllamaHost:   defaultHost,
		ScanDirs:     defaultScanDirs,
		FilePatterns: defaultFilePatterns,
		IdeaDirs:     defaultIdeaDirs,
	}

	if path, err := configPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, cfg) // best-effort; bad JSON just keeps defaults
			// Restore slice defaults if the file didn't include them
			if len(cfg.ScanDirs) == 0 {
				cfg.ScanDirs = defaultScanDirs
			}
			if len(cfg.FilePatterns) == 0 {
				cfg.FilePatterns = defaultFilePatterns
			}
			if len(cfg.IdeaDirs) == 0 {
				cfg.IdeaDirs = defaultIdeaDirs
			}
		}
	}

	if v := os.Getenv("SB_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		cfg.OllamaHost = v
	}

	if !strings.HasPrefix(cfg.OllamaHost, "http://") && !strings.HasPrefix(cfg.OllamaHost, "https://") {
		cfg.OllamaHost = "http://" + cfg.OllamaHost
	}

	return cfg
}

// Path returns the path to the config file, creating the dir if needed.
func Path() (string, error) {
	return configPath()
}

// configPath returns the path to the config file, creating the dir if needed.
func configPath() (string, error) {
	dir := filepath.Join(os.Getenv("HOME"), ".config", "sb")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// WriteDefaults writes a default config file if none exists.
func WriteDefaults() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	cfg := &Config{
		Model:        defaultModel,
		OllamaHost:   defaultHost,
		ScanDirs:     defaultScanDirs,
		FilePatterns: defaultFilePatterns,
		IdeaDirs:     defaultIdeaDirs,
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
