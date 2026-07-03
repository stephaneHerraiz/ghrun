package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds user settings for ghrun.
type Config struct {
	DefaultOrg             string        `yaml:"defaultOrg"`
	RefreshIntervalSeconds int           `yaml:"refreshIntervalSeconds"`
	RunListLimit           int           `yaml:"runListLimit"`
	ListPageSize           int           `yaml:"listPageSize"` // max rows shown at once in any list
	Favorites              []string      `yaml:"favorites"`    // "owner/name"
	Explain                ExplainConfig `yaml:"explain"`
}

// ExplainConfig configures the run-failure explanation feature (local RAG +
// Anthropic API / claude CLI). Every field is optional with a default.
type ExplainConfig struct {
	Enabled             *bool   `yaml:"enabled,omitempty"` // nil means enabled
	OllamaURL           string  `yaml:"ollamaURL"`
	EmbeddingModel      string  `yaml:"embeddingModel"`
	SimilarityThreshold float64 `yaml:"similarityThreshold"`
	AnthropicAPIKey     string  `yaml:"anthropicAPIKey"` // empty: read ANTHROPIC_API_KEY env
	Model               string  `yaml:"model"`
	ClaudeCmd           string  `yaml:"claudeCmd"`
	StorePath           string  `yaml:"storePath"` // empty: <config dir>/ghrun/explain-db
	MaxLogBytes         int     `yaml:"maxLogBytes"`
	Language            string  `yaml:"language"`
}

// IsEnabled reports whether explain is on. An unset flag means enabled, so
// existing config files without an explain section get the feature.
func (e ExplainConfig) IsEnabled() bool { return e.Enabled == nil || *e.Enabled }

// Default returns the baseline configuration.
func Default() Config {
	return Config{
		RefreshIntervalSeconds: 4,
		RunListLimit:           30,
		ListPageSize:           20,
		Explain: ExplainConfig{
			OllamaURL:           "http://localhost:11434",
			EmbeddingModel:      "nomic-embed-text",
			SimilarityThreshold: 0.86,
			Model:               "claude-sonnet-5",
			ClaudeCmd:           "claude",
			MaxLogBytes:         65536,
			Language:            "English",
		},
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
	if c.ListPageSize == 0 {
		c.ListPageSize = d.ListPageSize
	}
	if c.Explain.OllamaURL == "" {
		c.Explain.OllamaURL = d.Explain.OllamaURL
	}
	if c.Explain.EmbeddingModel == "" {
		c.Explain.EmbeddingModel = d.Explain.EmbeddingModel
	}
	if c.Explain.SimilarityThreshold == 0 {
		c.Explain.SimilarityThreshold = d.Explain.SimilarityThreshold
	}
	if c.Explain.Model == "" {
		c.Explain.Model = d.Explain.Model
	}
	if c.Explain.ClaudeCmd == "" {
		c.Explain.ClaudeCmd = d.Explain.ClaudeCmd
	}
	if c.Explain.MaxLogBytes == 0 {
		c.Explain.MaxLogBytes = d.Explain.MaxLogBytes
	}
	if c.Explain.Language == "" {
		c.Explain.Language = d.Explain.Language
	}
	return c
}

// LoadFrom reads config from path. A missing file is not an error: it yields Default().
func LoadFrom(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
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

// ResolveExplainStorePath returns the default explain knowledge-base
// directory (used when explain.storePath is empty).
func ResolveExplainStorePath() (string, error) {
	base, err := resolveBase("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ghrun", "explain-db"), nil
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
	if errors.Is(err, os.ErrNotExist) {
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
