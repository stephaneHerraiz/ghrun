package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
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
	_, err = tea.NewProgram(app, tea.WithAltScreen()).Run()
	return err
}
