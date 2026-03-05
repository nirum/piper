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

func TestApplyProfile(t *testing.T) {
	cfg := &Config{
		Provider: DefaultProvider,
		Model:    DefaultModel,
		Defaults: Defaults{MaxTokens: DefaultMaxTokens, System: DefaultSystem},
		Profiles: map[string]Profile{
			"coding": {
				Model: "claude-opus-4-20250514",
				Defaults: Defaults{
					System: "You are a senior engineer.",
				},
			},
			"quick": {
				Model:    "claude-haiku-4-5-20251001",
				Defaults: Defaults{MaxTokens: 512},
			},
		},
	}

	// Unknown profile returns false.
	if cfg.ApplyProfile("nonexistent") {
		t.Error("expected ApplyProfile to return false for unknown profile")
	}

	// Applying "coding" overrides model and system but not max_tokens.
	ok := cfg.ApplyProfile("coding")
	if !ok {
		t.Fatal("expected ApplyProfile to return true for 'coding'")
	}
	if cfg.Model != "claude-opus-4-20250514" {
		t.Errorf("model = %q, want claude-opus-4-20250514", cfg.Model)
	}
	if cfg.Defaults.System != "You are a senior engineer." {
		t.Errorf("system = %q, want 'You are a senior engineer.'", cfg.Defaults.System)
	}
	if cfg.Defaults.MaxTokens != DefaultMaxTokens {
		t.Errorf("max_tokens = %d, want %d (unchanged)", cfg.Defaults.MaxTokens, DefaultMaxTokens)
	}
}

func TestLoadProfilesFromFile(t *testing.T) {
	dir := t.TempDir()
	piperDir := filepath.Join(dir, "piper")
	os.MkdirAll(piperDir, 0700)
	cfgFile := filepath.Join(piperDir, "config.toml")

	content := `
provider = "anthropic"
model = "claude-sonnet-4-20250514"

[profiles.coding]
model = "claude-opus-4-20250514"

[profiles.coding.defaults]
system = "You are a senior engineer."

[profiles.quick]
model = "claude-haiku-4-5-20251001"

[profiles.quick.defaults]
max_tokens = 512
`
	if err := os.WriteFile(cfgFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := Load(os.Stderr)

	if len(cfg.Profiles) != 2 {
		t.Fatalf("profiles count = %d, want 2", len(cfg.Profiles))
	}

	codingProfile, ok := cfg.Profiles["coding"]
	if !ok {
		t.Fatal("expected 'coding' profile")
	}
	if codingProfile.Model != "claude-opus-4-20250514" {
		t.Errorf("coding model = %q, want claude-opus-4-20250514", codingProfile.Model)
	}
	if codingProfile.Defaults.System != "You are a senior engineer." {
		t.Errorf("coding system = %q, want 'You are a senior engineer.'", codingProfile.Defaults.System)
	}

	quickProfile := cfg.Profiles["quick"]
	if quickProfile.Defaults.MaxTokens != 512 {
		t.Errorf("quick max_tokens = %d, want 512", quickProfile.Defaults.MaxTokens)
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
