package config

import (
	"path/filepath"
	"testing"
)

func TestDirReturnsSBConfigDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir(): %v", err)
	}
	want := filepath.Join(home, ".config", "sb")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestActiveProviderLegacyOllamaCompat(t *testing.T) {
	t.Setenv("SB_PROVIDER", "")
	t.Setenv("SB_PROVIDER_TYPE", "")
	t.Setenv("SB_MODEL", "")
	t.Setenv("SB_BASE_URL", "")
	t.Setenv("SB_API_KEY", "")
	t.Setenv("SB_API_KEY_ENV", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := &Config{
		Model:      "llama3.2",
		OllamaHost: "localhost:11434",
	}

	p := cfg.ActiveProvider()
	if p.Type != "ollama" {
		t.Fatalf("type = %q, want ollama", p.Type)
	}
	if p.Model != "llama3.2" {
		t.Fatalf("model = %q, want legacy model", p.Model)
	}
	if p.BaseURL != "http://localhost:11434" {
		t.Fatalf("base_url = %q, want normalized legacy host", p.BaseURL)
	}
}

func TestActiveProviderNamedProfileWithEnvKeyFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	cfg := &Config{
		Provider: "remote",
		Providers: map[string]ProviderConfig{
			"remote": {
				Type:      "openai",
				Model:     "gpt-4.1-mini",
				BaseURL:   "https://api.openai.com/v1",
				APIKeyEnv: "OPENAI_API_KEY",
			},
		},
	}

	p := cfg.ActiveProvider()
	if p.Type != "openai" {
		t.Fatalf("type = %q, want openai", p.Type)
	}
	if got := p.ResolvedAPIKey(); got != "sk-test" {
		t.Fatalf("resolved key = %q, want env fallback", got)
	}
}

func TestActiveProviderEnvOverridesSelectedProfile(t *testing.T) {
	t.Setenv("SB_PROVIDER", "")
	t.Setenv("SB_PROVIDER_TYPE", "")
	t.Setenv("SB_MODEL", "claude-3-5-sonnet-latest")
	t.Setenv("SB_BASE_URL", "https://api.anthropic.com")
	t.Setenv("SB_API_KEY", "anthropic-key")
	t.Setenv("SB_API_KEY_ENV", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := &Config{
		Provider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {
				Type:    "anthropic",
				Model:   "claude-3-haiku",
				BaseURL: "https://example.invalid",
			},
		},
	}

	p := cfg.ActiveProvider()
	if p.Model != "claude-3-5-sonnet-latest" {
		t.Fatalf("model = %q, want SB_MODEL override", p.Model)
	}
	if p.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("base_url = %q, want SB_BASE_URL override", p.BaseURL)
	}
	if p.ResolvedAPIKey() != "anthropic-key" {
		t.Fatalf("resolved key = %q, want SB_API_KEY override", p.ResolvedAPIKey())
	}
}

func TestActiveProviderDefaultEnvLookupByType(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-env-key")

	p := ProviderConfig{Type: "anthropic"}
	if got := p.ResolvedAPIKey(); got != "anthropic-env-key" {
		t.Fatalf("resolved key = %q, want default provider env", got)
	}
}

func TestActiveProviderStatusDisabledWithoutCloudKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cfg := &Config{
		Provider: "openai",
		Providers: map[string]ProviderConfig{
			"openai": {
				Type:  "openai",
				Model: "gpt-test",
			},
		},
	}

	status := cfg.ActiveProviderStatus()
	if status.Enabled {
		t.Fatal("status enabled, want disabled")
	}
	if status.Problem != "missing API key" {
		t.Fatalf("problem = %q, want missing API key", status.Problem)
	}
}

func TestActiveProviderStatusEnabledForOllama(t *testing.T) {
	cfg := &Config{
		Provider: "ollama",
		Providers: map[string]ProviderConfig{
			"ollama": {
				Type:    "ollama",
				Model:   "qwen2.5:7b",
				BaseURL: "http://localhost:11434",
			},
		},
	}

	status := cfg.ActiveProviderStatus()
	if !status.Enabled {
		t.Fatalf("status disabled with problem %q, want enabled", status.Problem)
	}
	if status.Name != "ollama" {
		t.Fatalf("name = %q, want ollama", status.Name)
	}
}
