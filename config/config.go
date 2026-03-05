package config

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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
}

type Defaults struct {
	MaxTokens int           `toml:"max_tokens"`
	System    string        `toml:"system"`
	Timeout   time.Duration `toml:"timeout"`
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
