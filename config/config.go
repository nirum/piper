package config

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	DefaultProvider  = "anthropic"
	DefaultModel     = "claude-sonnet-4-20250514"
	DefaultMaxTokens = 4096
	DefaultSystem    = "You are a helpful assistant."
)

type Config struct {
	Provider string `toml:"provider"`
	Model    string `toml:"model"`
	APIKey   string `toml:"api_key"`
	BaseURL  string `toml:"base_url"`

	Defaults Defaults `toml:"defaults"`
	Profiles map[string]Profile `toml:"profiles"`
}

type Defaults struct {
	MaxTokens int    `toml:"max_tokens"`
	System    string `toml:"system"`
}

// Profile holds per-profile overrides. Zero values mean "use the top-level default".
type Profile struct {
	Provider  string   `toml:"provider"`
	Model     string   `toml:"model"`
	APIKey    string   `toml:"api_key"`
	BaseURL   string   `toml:"base_url"`
	Defaults  Defaults `toml:"defaults"`
}

// ApplyProfile merges the named profile's non-zero fields into cfg.
// Returns false if the profile name is not found.
func (c *Config) ApplyProfile(name string) bool {
	p, ok := c.Profiles[name]
	if !ok {
		return false
	}
	if p.Provider != "" {
		c.Provider = p.Provider
	}
	if p.Model != "" {
		c.Model = p.Model
	}
	if p.APIKey != "" {
		c.APIKey = p.APIKey
	}
	if p.BaseURL != "" {
		c.BaseURL = p.BaseURL
	}
	if p.Defaults.MaxTokens != 0 {
		c.Defaults.MaxTokens = p.Defaults.MaxTokens
	}
	if p.Defaults.System != "" {
		c.Defaults.System = p.Defaults.System
	}
	return true
}

// Load reads config from the standard path (~/.config/piper/config.toml).
// Missing file is not an error; all fields have defaults.
func Load(stderr io.Writer) *Config {
	cfg := &Config{
		Provider: DefaultProvider,
		Model:    DefaultModel,
		Defaults: Defaults{
			MaxTokens: DefaultMaxTokens,
			System:    DefaultSystem,
		},
	}

	path := configPath()
	if path == "" {
		return cfg
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return cfg
	}
	if err != nil {
		fmt.Fprintf(stderr, "piper: warning: cannot stat config: %v\n", err)
		return cfg
	}

	// Warn if config file permissions are too open.
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		fmt.Fprintf(stderr, "piper: warning: %s has permissions %04o, should be 0600\n", path, mode)
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		fmt.Fprintf(stderr, "piper: warning: cannot parse config: %v\n", err)
	}

	return cfg
}

// ResolveAPIKey returns the API key for the given provider using the
// resolution order: env var → config file.
func (c *Config) ResolveAPIKey(provider string) string {
	switch provider {
	case "anthropic":
		if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			return key
		}
	case "openai":
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			return key
		}
	}
	return c.APIKey
}

func configPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "piper", "config.toml")
}

// ConfigPath is exported for testing.
func ConfigPath() string {
	return configPath()
}

// CheckPermissions checks if the file at path has permissions wider than 0600.
func CheckPermissions(path string) (fs.FileMode, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	mode := info.Mode().Perm()
	return mode, mode&0o077 != 0
}
