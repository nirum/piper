package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// With no config file, defaults should be used.
	cfg := Load(os.Stderr)
	if cfg.Provider != DefaultProvider {
		t.Errorf("provider = %q, want %q", cfg.Provider, DefaultProvider)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("model = %q, want %q", cfg.Model, DefaultModel)
	}
	if cfg.Defaults.MaxTokens != DefaultMaxTokens {
		t.Errorf("max_tokens = %d, want %d", cfg.Defaults.MaxTokens, DefaultMaxTokens)
	}
	if cfg.Defaults.System != DefaultSystem {
		t.Errorf("system = %q, want %q", cfg.Defaults.System, DefaultSystem)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.toml")

	content := `
provider = "openai"
model = "gpt-4o"
api_key = "sk-test-123"
base_url = "http://localhost:11434"

[defaults]
max_tokens = 2048
system = "Be concise."
`
	if err := os.WriteFile(cfgFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	// Override XDG_CONFIG_HOME to use our temp dir.
	t.Setenv("XDG_CONFIG_HOME", dir)
	// The config file needs to be at $XDG_CONFIG_HOME/piper/config.toml
	piperDir := filepath.Join(dir, "piper")
	os.MkdirAll(piperDir, 0700)
	os.Rename(cfgFile, filepath.Join(piperDir, "config.toml"))

	cfg := Load(os.Stderr)
	if cfg.Provider != "openai" {
		t.Errorf("provider = %q, want openai", cfg.Provider)
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", cfg.Model)
	}
	if cfg.APIKey != "sk-test-123" {
		t.Errorf("api_key = %q, want sk-test-123", cfg.APIKey)
	}
	if cfg.BaseURL != "http://localhost:11434" {
		t.Errorf("base_url = %q, want http://localhost:11434", cfg.BaseURL)
	}
	if cfg.Defaults.MaxTokens != 2048 {
		t.Errorf("max_tokens = %d, want 2048", cfg.Defaults.MaxTokens)
	}
	if cfg.Defaults.System != "Be concise." {
		t.Errorf("system = %q, want Be concise.", cfg.Defaults.System)
	}
}

func TestResolveAPIKey_EnvOverridesConfig(t *testing.T) {
	cfg := &Config{APIKey: "config-key"}

	// Anthropic: env wins.
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	if got := cfg.ResolveAPIKey("anthropic"); got != "env-key" {
		t.Errorf("ResolveAPIKey(anthropic) = %q, want env-key", got)
	}

	// Fall back to config.
	t.Setenv("ANTHROPIC_API_KEY", "")
	if got := cfg.ResolveAPIKey("anthropic"); got != "config-key" {
		t.Errorf("ResolveAPIKey(anthropic) = %q, want config-key", got)
	}

	// OpenAI: env wins.
	t.Setenv("OPENAI_API_KEY", "openai-env")
	if got := cfg.ResolveAPIKey("openai"); got != "openai-env" {
		t.Errorf("ResolveAPIKey(openai) = %q, want openai-env", got)
	}
}

func TestCheckPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// 0644 — too open.
	os.WriteFile(path, []byte("test"), 0644)
	mode, tooOpen := CheckPermissions(path)
	if !tooOpen {
		t.Errorf("expected permissions %04o to be flagged as too open", mode)
	}

	// 0600 — correct.
	os.Chmod(path, 0600)
	mode, tooOpen = CheckPermissions(path)
	if tooOpen {
		t.Errorf("expected permissions %04o to be acceptable", mode)
	}
}
