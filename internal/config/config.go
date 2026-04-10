package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config holds sb runtime settings. Load order: defaults → config file → env vars.
type Config struct {
	Model      string `json:"model"`
	OllamaHost string `json:"ollama_host"`
}

const defaultModel = "qwen2.5:7b"
const defaultHost = "http://localhost:11434"

// Load reads ~/.config/sb/config.json (if present), then applies env var overrides.
func Load() *Config {
	cfg := &Config{
		Model:      defaultModel,
		OllamaHost: defaultHost,
	}

	if path, err := configPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, cfg) // best-effort; bad JSON just keeps defaults
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
	cfg := &Config{Model: defaultModel, OllamaHost: defaultHost}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
