package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
	"github.com/stephaneHerraiz/ghrun/internal/ui"
)

// ensureConfig writes a starter config on first run, then loads it.
func ensureConfig() (config.Config, error) {
	p, err := config.ResolveConfigPath()
	if err != nil {
		return config.Config{}, err
	}
	if _, statErr := os.Stat(p); statErr != nil {
		// A real stat error (e.g. permission denied) must surface, not be
		// silently treated as "file absent" — only ErrNotExist means first run.
		if !errors.Is(statErr, os.ErrNotExist) {
			return config.Config{}, statErr
		}
		template := config.Default()
		template.Favorites = []string{}
		if err := config.SaveTo(p, template); err != nil {
			return config.Config{}, err
		}
	}
	return config.LoadFrom(p)
}

// run is the real entrypoint, returning an error for testability.
func run() error {
	client := gh.NewClient(gh.NewGHRunner())
	if err := client.AuthStatus(); err != nil {
		return fmt.Errorf("gh not authenticated — run `gh auth login`:\n%w", err)
	}
	cfg, err := ensureConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	app := ui.NewApp(client, cfg)
	if svc := buildExplainService(cfg); svc != nil {
		app = app.WithExplainService(svc)
	}
	_, err = tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

// buildExplainService assembles the run-failure explanation service from
// config, or nil when the feature is disabled. Explainer order encodes the
// fallback: Anthropic API when a key is configured, then the claude CLI when
// the binary is on PATH. Every part degrades gracefully at runtime, so
// construction never fails.
func buildExplainService(cfg config.Config) ui.ExplainService {
	ec := cfg.Explain
	if !ec.IsEnabled() {
		return nil
	}
	storePath := expandHome(ec.StorePath)
	if storePath == "" {
		if p, err := config.ResolveExplainStorePath(); err == nil {
			storePath = p
		}
	}
	var store explain.Store
	if storePath != "" {
		if s, err := explain.NewChromemStore(storePath); err == nil {
			store = s
		}
	}
	var explainers []explain.Explainer
	key := ec.AnthropicAPIKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if key != "" {
		explainers = append(explainers, explain.NewAnthropicExplainer(key, ec.Model))
	}
	if _, err := exec.LookPath(ec.ClaudeCmd); err == nil {
		explainers = append(explainers, explain.NewClaudeCLIExplainer(ec.ClaudeCmd))
	}
	return explain.NewService(
		explain.NewOllamaEmbedder(ec.OllamaURL, ec.EmbeddingModel),
		&explain.Chain{Explainers: explainers},
		store,
		explain.Options{
			Threshold:   float32(ec.SimilarityThreshold),
			MaxLogBytes: ec.MaxLogBytes,
			Language:    ec.Language,
		},
	)
}

// expandHome resolves a leading "~/" so the documented storePath example
// works verbatim; chromem would otherwise create a literal "~" directory.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
