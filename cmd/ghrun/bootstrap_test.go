package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stephaneHerraiz/ghrun/internal/config"
)

func TestEnsureConfigWritesTemplateOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := ensureConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RefreshIntervalSeconds != 4 {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	p := filepath.Join(dir, "ghrun", "config.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("template not written: %v", err)
	}
	// second call must not error and must load the same file
	if _, err := ensureConfig(); err != nil {
		t.Fatalf("second ensureConfig: %v", err)
	}
	if cfg.DefaultOrg != "" {
		t.Errorf("first-run DefaultOrg = %q, want empty (org chosen interactively)", cfg.DefaultOrg)
	}
	_ = config.Default()
}
