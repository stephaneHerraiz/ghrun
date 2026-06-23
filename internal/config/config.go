package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds user settings for ghrun.
type Config struct {
	DefaultOrg             string   `yaml:"defaultOrg"`
	RefreshIntervalSeconds int      `yaml:"refreshIntervalSeconds"`
	RunListLimit           int      `yaml:"runListLimit"`
	Favorites              []string `yaml:"favorites"` // "owner/name"
}

// Default returns the baseline configuration.
func Default() Config {
	return Config{
		RefreshIntervalSeconds: 4,
		RunListLimit:           30,
	}
}

// applyDefaults fills zero-valued numeric fields with their defaults.
func applyDefaults(c Config) Config {
	d := Default()
	if c.RefreshIntervalSeconds == 0 {
		c.RefreshIntervalSeconds = d.RefreshIntervalSeconds
	}
	if c.RunListLimit == 0 {
		c.RunListLimit = d.RunListLimit
	}
	return c
}

// LoadFrom reads config from path. A missing file is not an error: it yields Default().
func LoadFrom(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	return applyDefaults(c), nil
}

// SaveTo writes config to path, creating parent directories.
func SaveTo(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func resolveBase(envVar, fallbackSub string) (string, error) {
	if v := os.Getenv(envVar); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallbackSub), nil
}

// ResolveConfigPath returns the YAML config file path.
func ResolveConfigPath() (string, error) {
	base, err := resolveBase("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ghrun", "config.yaml"), nil
}

// ResolveCachePath returns the repo cache file path.
func ResolveCachePath() (string, error) {
	base, err := resolveBase("XDG_CACHE_HOME", ".cache")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ghrun", "repos.json"), nil
}

// Load reads config from the resolved path.
func Load() (Config, error) {
	p, err := ResolveConfigPath()
	if err != nil {
		return Config{}, err
	}
	return LoadFrom(p)
}

// Save writes config to the resolved path.
func (c Config) Save() error {
	p, err := ResolveConfigPath()
	if err != nil {
		return err
	}
	return SaveTo(p, c)
}

// LoadRepoCacheFrom reads the cached repo list (JSON array of "owner/name").
func LoadRepoCacheFrom(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var repos []string
	if err := json.Unmarshal(b, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// SaveRepoCacheTo writes the repo list to path as JSON.
func SaveRepoCacheTo(path string, repos []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
