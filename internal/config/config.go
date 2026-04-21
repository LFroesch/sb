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
	Name string `json:"name"` // slug routing uses to refer to this target
	Path string `json:"path"` // WORK.md path; ~ is expanded
}

// ScanRoot names a recursive discovery root. Name is used in fallback labels
// when sb needs extra context to disambiguate same-named files.
type ScanRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ProviderConfig describes one named LLM profile.
type ProviderConfig struct {
	Type      string            `json:"type"`                  // ollama|openai|anthropic
	Model     string            `json:"model,omitempty"`       // provider model id
	BaseURL   string            `json:"base_url,omitempty"`    // base API URL
	APIKey    string            `json:"api_key,omitempty"`     // optional inline secret
	APIKeyEnv string            `json:"api_key_env,omitempty"` // preferred env var for secrets
	Headers   map[string]string `json:"headers,omitempty"`     // optional provider-specific headers
}

// ProviderStatus is a lightweight UI-facing summary of the active LLM setup.
type ProviderStatus struct {
	Name    string
	Type    string
	Model   string
	Enabled bool
	Problem string
}

// ResolvedAPIKey returns the inline key or falls back to the configured env var.
func (p ProviderConfig) ResolvedAPIKey() string {
	if p.APIKey != "" {
		return p.APIKey
	}
	envName := p.APIKeyEnv
	if envName == "" {
		envName = defaultAPIKeyEnv(p.Type)
	}
	if envName == "" {
		return ""
	}
	return os.Getenv(envName)
}

// Config holds sb runtime settings. Load order: defaults → config file → env vars.
type Config struct {
	Provider       string                    `json:"provider,omitempty"` // active provider profile name
	Providers      map[string]ProviderConfig `json:"providers,omitempty"`
	Model          string                    `json:"model"`                     // deprecated compatibility field
	OllamaHost     string                    `json:"ollama_host"`               // deprecated compatibility field
	ScanRoots      []ScanRoot                `json:"scan_roots,omitempty"`      // recursive discovery roots
	ScanDirs       []string                  `json:"scan_dirs,omitempty"`       // deprecated compatibility field
	FilePatterns   []string                  `json:"file_patterns"`             // file names to match (e.g. WORK.md, ROADMAP.md)
	IdeaDirs       []string                  `json:"idea_dirs"`                 // dirs where all .md files are loaded flat
	LabelMaxDepth  int                       `json:"label_max_depth,omitempty"` // number of trailing path components to keep for fallback labels
	IndexPath      string                    `json:"index_path"`                // path to the auto-regenerated routing-context index
	LogLevel       string                    `json:"log_level,omitempty"`       // slog level: debug|info|warn|error
	CatchallTarget *Target                   `json:"catchall_target,omitempty"` // optional generic-notes bucket
	IdeasTarget    *Target                   `json:"ideas_target,omitempty"`    // optional ideas bucket
}

const (
	defaultModel         = "qwen2.5:7b"
	defaultOllamaHost    = "http://localhost:11434"
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultAnthropicURL  = "https://api.anthropic.com"
	defaultIndexPath     = "~/.config/sb/index.md"
	defaultLabelMaxDepth = 2
	defaultLogLevel      = "info"
	defaultProviderName  = "ollama"
)

var defaultScanRoots = []ScanRoot{{Name: "projects", Path: "~/projects"}}
var defaultFilePatterns = []string{"WORK.md"}

func defaultProviders() map[string]ProviderConfig {
	return map[string]ProviderConfig{
		defaultProviderName: {
			Type:    "ollama",
			Model:   defaultModel,
			BaseURL: defaultOllamaHost,
		},
	}
}

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

// ActiveProvider returns the selected provider profile with defaults and env
// overrides applied. Existing top-level model/ollama_host configs continue to
// map to the implicit "ollama" profile.
func (c *Config) ActiveProvider() ProviderConfig {
	providers := c.normalizedProviders()
	name := strings.TrimSpace(c.Provider)
	if name == "" {
		name = defaultProviderName
	}
	p, ok := providers[name]
	if !ok {
		p = providers[defaultProviderName]
	}

	if v := os.Getenv("SB_PROVIDER_TYPE"); v != "" {
		p.Type = strings.TrimSpace(v)
	}
	if v := os.Getenv("SB_MODEL"); v != "" {
		p.Model = v
	}
	if v := os.Getenv("SB_BASE_URL"); v != "" {
		p.BaseURL = v
	}
	if v := os.Getenv("SB_API_KEY"); v != "" {
		p.APIKey = v
	}
	if v := os.Getenv("SB_API_KEY_ENV"); v != "" {
		p.APIKeyEnv = v
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" && strings.EqualFold(p.Type, "ollama") {
		p.BaseURL = v
	}

	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	if p.Type == "" {
		p.Type = "ollama"
	}
	if p.Model == "" {
		p.Model = defaultModel
	}
	if p.BaseURL == "" {
		p.BaseURL = defaultBaseURL(p.Type)
	}
	if !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
		p.BaseURL = "http://" + p.BaseURL
	}
	if p.Headers == nil {
		p.Headers = map[string]string{}
	}
	return p
}

// ActiveProviderStatus reports whether the selected provider is configured well
// enough for sb to attempt requests. It does not make any network calls.
func (c *Config) ActiveProviderStatus() ProviderStatus {
	name := strings.TrimSpace(c.Provider)
	if name == "" {
		name = defaultProviderName
	}
	p := c.ActiveProvider()
	status := ProviderStatus{
		Name:    name,
		Type:    p.Type,
		Model:   p.Model,
		Enabled: true,
	}

	if p.Type == "" {
		status.Enabled = false
		status.Problem = "no provider type configured"
		return status
	}
	if p.Model == "" {
		status.Enabled = false
		status.Problem = "no model configured"
		return status
	}

	switch p.Type {
	case "openai", "anthropic":
		if p.ResolvedAPIKey() == "" {
			status.Enabled = false
			if p.APIKeyEnv != "" {
				status.Problem = "missing " + p.APIKeyEnv
			} else {
				status.Problem = "missing API key"
			}
		}
	case "ollama":
		if strings.TrimSpace(p.BaseURL) == "" {
			status.Enabled = false
			status.Problem = "missing base URL"
		}
	default:
		status.Enabled = false
		status.Problem = "unsupported provider type"
	}

	return status
}

func (c *Config) normalizedProviders() map[string]ProviderConfig {
	providers := cloneProviders(c.Providers)
	if len(providers) == 0 {
		providers = defaultProviders()
	}

	legacy := providers[defaultProviderName]
	if legacy.Type == "" {
		legacy.Type = "ollama"
	}
	if c.Model != "" {
		legacy.Model = c.Model
	}
	if c.OllamaHost != "" {
		legacy.BaseURL = c.OllamaHost
	}
	if legacy.Model == "" {
		legacy.Model = defaultModel
	}
	if legacy.BaseURL == "" {
		legacy.BaseURL = defaultOllamaHost
	}
	providers[defaultProviderName] = legacy

	for name, p := range providers {
		p.Type = strings.ToLower(strings.TrimSpace(p.Type))
		if p.Type == "" {
			p.Type = name
		}
		if p.Model == "" {
			p.Model = defaultModel
		}
		if p.BaseURL == "" {
			p.BaseURL = defaultBaseURL(p.Type)
		}
		providers[name] = p
	}
	return providers
}

func cloneProviders(in map[string]ProviderConfig) map[string]ProviderConfig {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ProviderConfig, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func defaultBaseURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "openai":
		return defaultOpenAIBaseURL
	case "anthropic":
		return defaultAnthropicURL
	default:
		return defaultOllamaHost
	}
}

func defaultAPIKeyEnv(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	default:
		return ""
	}
}

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
		Provider:      defaultProviderName,
		Providers:     defaultProviders(),
		Model:         defaultModel,
		OllamaHost:    defaultOllamaHost,
		ScanRoots:     defaultScanRoots,
		FilePatterns:  defaultFilePatterns,
		LabelMaxDepth: defaultLabelMaxDepth,
		IndexPath:     defaultIndexPath,
		LogLevel:      defaultLogLevel,
	}

	if path, err := configPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, cfg) // best-effort; bad JSON just keeps defaults
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
			if strings.TrimSpace(cfg.Provider) == "" {
				cfg.Provider = defaultProviderName
			}
			if len(cfg.Providers) == 0 {
				cfg.Providers = defaultProviders()
			}
		}
	}

	if v := os.Getenv("SB_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if cfg.LabelMaxDepth <= 0 {
		cfg.LabelMaxDepth = defaultLabelMaxDepth
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = defaultProviderName
	}
	if len(cfg.Providers) == 0 {
		cfg.Providers = defaultProviders()
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
		Provider:      defaultProviderName,
		Providers:     defaultProviders(),
		Model:         defaultModel,
		OllamaHost:    defaultOllamaHost,
		ScanRoots:     defaultScanRoots,
		FilePatterns:  defaultFilePatterns,
		LabelMaxDepth: defaultLabelMaxDepth,
		IndexPath:     defaultIndexPath,
		LogLevel:      defaultLogLevel,
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
