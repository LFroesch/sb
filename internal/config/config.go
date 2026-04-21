package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Target names a special routing destination — a "catch-all" or "ideas bucket"
// that brain-dumped items can be routed to when they don't fit a real project.
// Path is the WORK.md file the item gets appended to (~ allowed).
type Target struct {
	Name string `json:"name"` // slug ollama uses to refer to this target
	Path string `json:"path"` // WORK.md path; ~ is expanded
}

// ScanRoot names a recursive discovery root. Name is used in fallback labels
// when sb needs extra context to disambiguate same-named files.
type ScanRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Config holds sb runtime settings. Load order: defaults → config file → env vars.
type Config struct {
	Model          string     `json:"model"`
	OllamaHost     string     `json:"ollama_host"`
	ScanRoots      []ScanRoot `json:"scan_roots,omitempty"`      // recursive discovery roots
	ScanDirs       []string   `json:"scan_dirs,omitempty"`       // deprecated compatibility field
	FilePatterns   []string   `json:"file_patterns"`             // file names to match (e.g. WORK.md, ROADMAP.md)
	IdeaDirs       []string   `json:"idea_dirs"`                 // dirs where all .md files are loaded flat
	LabelMaxDepth  int        `json:"label_max_depth,omitempty"` // number of trailing path components to keep for fallback labels
	IndexPath      string     `json:"index_path"`                // path to the auto-regenerated routing-context index
	LogLevel       string     `json:"log_level,omitempty"`       // slog level: debug|info|warn|error
	CatchallTarget *Target    `json:"catchall_target,omitempty"` // optional generic-notes bucket
	IdeasTarget    *Target    `json:"ideas_target,omitempty"`    // optional ideas bucket
}

const defaultModel = "qwen2.5:7b"
const defaultHost = "http://localhost:11434"
const defaultIndexPath = "~/.config/sb/index.md"
const defaultLabelMaxDepth = 2
const defaultLogLevel = "info"

var defaultScanRoots = []ScanRoot{{Name: "projects", Path: "~/projects"}}
var defaultFilePatterns = []string{"WORK.md"}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), p[2:])
	}
	return p
}

// ExpandedScanRoots returns ScanRoots with ~ expanded.
func (c *Config) ExpandedScanRoots() []ScanRoot {
	roots := c.ScanRoots
	if len(roots) == 0 {
		roots = scanRootsFromDirs(c.ScanDirs)
	}
	if len(roots) == 0 {
		roots = defaultScanRoots
	}
	out := make([]ScanRoot, 0, len(roots))
	for _, root := range roots {
		path := expandHome(root.Path)
		if path == "" {
			continue
		}
		name := strings.TrimSpace(root.Name)
		if name == "" {
			name = inferRootName(path)
		}
		out = append(out, ScanRoot{Name: name, Path: path})
	}
	return out
}

// ExpandedIdeaDirs returns IdeaDirs with ~ expanded.
func (c *Config) ExpandedIdeaDirs() []string { return expandAll(c.IdeaDirs) }

// ExpandedIndexPath returns IndexPath with ~ expanded.
func (c *Config) ExpandedIndexPath() string { return expandHome(c.IndexPath) }

// ExpandHome is the package's ~ expander, exported for callers that need to
// expand a Target's Path or other config-supplied paths.
func ExpandHome(p string) string { return expandHome(p) }

func expandAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = expandHome(s)
	}
	return out
}

func inferRootName(path string) string {
	base := filepath.Base(strings.TrimRight(path, string(filepath.Separator)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "root"
	}
	return base
}

func scanRootsFromDirs(dirs []string) []ScanRoot {
	out := make([]ScanRoot, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		path := expandHome(dir)
		out = append(out, ScanRoot{
			Name: inferRootName(path),
			Path: path,
		})
	}
	return out
}

// Load reads ~/.config/sb/config.json (if present), then applies env var overrides.
func Load() *Config {
	cfg := &Config{
		Model:         defaultModel,
		OllamaHost:    defaultHost,
		ScanRoots:     defaultScanRoots,
		FilePatterns:  defaultFilePatterns,
		LabelMaxDepth: defaultLabelMaxDepth,
		IndexPath:     defaultIndexPath,
		LogLevel:      defaultLogLevel,
	}

	if path, err := configPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, cfg) // best-effort; bad JSON just keeps defaults
			// Restore scalar/slice defaults if the file didn't include them
			if len(cfg.ScanRoots) == 0 && len(cfg.ScanDirs) > 0 {
				cfg.ScanRoots = scanRootsFromDirs(cfg.ScanDirs)
			}
			if len(cfg.ScanRoots) == 0 {
				cfg.ScanRoots = defaultScanRoots
			}
			if len(cfg.FilePatterns) == 0 {
				cfg.FilePatterns = defaultFilePatterns
			}
			if cfg.IndexPath == "" {
				cfg.IndexPath = defaultIndexPath
			}
			if cfg.LabelMaxDepth <= 0 {
				cfg.LabelMaxDepth = defaultLabelMaxDepth
			}
			if cfg.LogLevel == "" {
				cfg.LogLevel = defaultLogLevel
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
	if cfg.LabelMaxDepth <= 0 {
		cfg.LabelMaxDepth = defaultLabelMaxDepth
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
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
		Model:         defaultModel,
		OllamaHost:    defaultHost,
		ScanRoots:     defaultScanRoots,
		FilePatterns:  defaultFilePatterns,
		LabelMaxDepth: defaultLabelMaxDepth,
		IndexPath:     defaultIndexPath,
		LogLevel:      defaultLogLevel,
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
