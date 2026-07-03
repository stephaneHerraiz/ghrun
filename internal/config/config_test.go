package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultValues(t *testing.T) {
	d := Default()
	if d.RefreshIntervalSeconds != 4 {
		t.Errorf("RefreshIntervalSeconds = %d, want 4", d.RefreshIntervalSeconds)
	}
	if d.RunListLimit != 30 {
		t.Errorf("RunListLimit = %d, want 30", d.RunListLimit)
	}
	if d.ListPageSize != 20 {
		t.Errorf("ListPageSize = %d, want 20", d.ListPageSize)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	got, err := LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom missing = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Errorf("LoadFrom missing = %+v, want Default() %+v", got, Default())
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.yaml")
	in := Config{
		DefaultOrg:             "stephaneHerraiz",
		RefreshIntervalSeconds: 6,
		RunListLimit:           50,
		ListPageSize:           25,
		Favorites:              []string{"stephaneHerraiz/ghrun"},
		Explain:                Default().Explain,
	}
	if err := SaveTo(p, in); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	got, err := LoadFrom(p)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip = %+v, want %+v", got, in)
	}
}

func TestLoadAppliesDefaultsForZeroFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	// only DefaultOrg set; numeric fields zero -> defaults applied on load
	if err := SaveTo(p, Config{DefaultOrg: "acme"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.RefreshIntervalSeconds != 4 || got.RunListLimit != 30 || got.ListPageSize != 20 {
		t.Errorf("defaults not applied: %+v", got)
	}
}

func TestRepoCacheRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "repos.json")
	in := []string{"a/b", "c/d"}
	if err := SaveRepoCacheTo(p, in); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRepoCacheFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("cache round trip = %v, want %v", got, in)
	}
}

func TestExplainDefaults(t *testing.T) {
	c := Default().Explain
	if !c.IsEnabled() {
		t.Error("explain must default to enabled")
	}
	if c.OllamaURL != "http://localhost:11434" || c.EmbeddingModel != "nomic-embed-text" ||
		c.SimilarityThreshold != 0.86 || c.Model != "claude-sonnet-5" ||
		c.ClaudeCmd != "claude" || c.MaxLogBytes != 65536 || c.Language != "English" {
		t.Errorf("defaults = %+v", c)
	}
}

func TestExplainSectionAbsentGetsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("defaultOrg: acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Explain.IsEnabled() || c.Explain.SimilarityThreshold != 0.86 {
		t.Errorf("explain = %+v", c.Explain)
	}
}

func TestExplainPartialSectionFillsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "explain:\n  enabled: false\n  model: claude-opus-4-8\n"
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Explain.IsEnabled() {
		t.Error("enabled: false must stick")
	}
	if c.Explain.Model != "claude-opus-4-8" {
		t.Errorf("model = %q", c.Explain.Model)
	}
	if c.Explain.OllamaURL != "http://localhost:11434" || c.Explain.MaxLogBytes != 65536 {
		t.Errorf("unset fields must get defaults: %+v", c.Explain)
	}
}

func TestResolveExplainStorePath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	p, err := ResolveExplainStorePath()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/xdg/ghrun/explain-db" {
		t.Errorf("path = %q", p)
	}
}
