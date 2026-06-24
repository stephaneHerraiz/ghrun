package config

import (
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
