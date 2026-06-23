package ui

import (
	"testing"

	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestLaunchValidateFlagsRequiredEmpty(t *testing.T) {
	inputs := []gh.Input{
		{Name: "environment", Type: gh.InputChoice, Required: true, Options: []string{"staging", "production"}},
		{Name: "version", Type: gh.InputString, Default: "1.0.0"},
		{Name: "token", Type: gh.InputString, Required: true}, // empty -> invalid
	}
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs)
	// environment defaults to first option (staging) so it's satisfied; token is empty.
	missing := l.validate()
	if len(missing) != 1 || missing[0] != "token" {
		t.Fatalf("missing = %v, want [token]", missing)
	}
}

func TestLaunchValuesUseDefaults(t *testing.T) {
	inputs := []gh.Input{
		{Name: "version", Type: gh.InputString, Default: "2.3.4"},
		{Name: "dry_run", Type: gh.InputBoolean, Default: "true"},
	}
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs)
	vals := l.values()
	if vals["version"] != "2.3.4" || vals["dry_run"] != "true" {
		t.Fatalf("values = %v", vals)
	}
}

func TestLaunchChoiceDefaultsToFirstOption(t *testing.T) {
	inputs := []gh.Input{
		{Name: "env", Type: gh.InputChoice, Required: true, Options: []string{"staging", "production"}},
	}
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs)
	if l.values()["env"] != "staging" {
		t.Fatalf("choice default = %q, want staging", l.values()["env"])
	}
}
