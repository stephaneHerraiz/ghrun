package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestLaunchBranchListWindowsAndMouseScrolls(t *testing.T) {
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, nil, 3) // page size 3
	branches := []string{"zzz0", "zzz1", "zzz2", "zzz3", "zzz4", "zzz5"}
	s, _ := l.Update(branchesLoadedMsg{branches: branches})
	l = s.(*launch)

	v := l.View()
	if strings.Count(v, "zzz") != 3 {
		t.Fatalf("expected 3 branch rows shown (page size 3), got %d:\n%s", strings.Count(v, "zzz"), v)
	}
	if !strings.Contains(v, scrollThumb) {
		t.Errorf("overflowing branch list should render a scrollbar; got:\n%s", v)
	}

	// Mouse wheel down scrolls the branch list.
	for range 4 {
		s, _ = l.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
		l = s.(*launch)
	}
	if l.branchList.cursor != 4 || l.branchList.offset != 2 {
		t.Fatalf("cursor/offset = %d/%d after 4 wheel-down, want 4/2", l.branchList.cursor, l.branchList.offset)
	}
}

func TestLaunchPreselectsDefaultBranch(t *testing.T) {
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, nil, 20)
	// "main" is not first alphabetically, but should be pre-selected.
	s, _ := l.Update(branchesLoadedMsg{branches: []string{"alpha", "main", "zeta"}})
	l = s.(*launch)
	if l.currentBranch() != "main" {
		t.Fatalf("currentBranch = %q, want main (default pre-selected)", l.currentBranch())
	}
}

// TestLaunchEnterSubmits guards the fix for ctrl+s being swallowed by terminal
// flow control: Enter must launch the workflow (reliable in every terminal).
func TestLaunchEnterSubmits(t *testing.T) {
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, nil, 20)
	l.phase = phaseInputs
	s, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	l = s.(*launch)
	if l.phase != phaseSubmitting {
		t.Fatalf("phase = %v after Enter, want phaseSubmitting", l.phase)
	}
	if cmd == nil {
		t.Fatal("Enter should return a dispatch command")
	}
}

// TestLaunchEnterBlockedByMissingRequired verifies Enter still routes through
// validation: a missing required field blocks the launch instead of dispatching.
func TestLaunchEnterBlockedByMissingRequired(t *testing.T) {
	inputs := []gh.Input{{Name: "token", Type: gh.InputString, Required: true}}
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs, 20)
	l.phase = phaseInputs
	l.focusCurrent()
	s, _ := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	l = s.(*launch)
	if l.phase != phaseInputs {
		t.Fatalf("phase = %v, want phaseInputs (blocked by missing field)", l.phase)
	}
	if len(l.missing) != 1 || l.missing[0] != "token" {
		t.Fatalf("missing = %v, want [token]", l.missing)
	}
}

func TestLaunchValidateFlagsRequiredEmpty(t *testing.T) {
	inputs := []gh.Input{
		{Name: "environment", Type: gh.InputChoice, Required: true, Options: []string{"staging", "production"}},
		{Name: "version", Type: gh.InputString, Default: "1.0.0"},
		{Name: "token", Type: gh.InputString, Required: true}, // empty -> invalid
	}
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs, 20)
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
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs, 20)
	vals := l.values()
	if vals["version"] != "2.3.4" || vals["dry_run"] != "true" {
		t.Fatalf("values = %v", vals)
	}
}

func TestLaunchChoiceDefaultsToFirstOption(t *testing.T) {
	inputs := []gh.Input{
		{Name: "env", Type: gh.InputChoice, Required: true, Options: []string{"staging", "production"}},
	}
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs, 20)
	if l.values()["env"] != "staging" {
		t.Fatalf("choice default = %q, want staging", l.values()["env"])
	}
}

// TestLaunchFindRunRetryHandler exercises the runFoundMsg branch of Update
// (the dispatch→find-run polling loop) without real sleeps: it drives the
// handler with crafted messages and inspects the returned command/phase.
// None of these paths invoke the client synchronously, so a nil client is fine.
func TestLaunchFindRunRetryHandler(t *testing.T) {
	newSubmitting := func() *launch {
		l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, nil, 20)
		l.phase = phaseSubmitting
		return l
	}

	// Run not found yet, attempts remaining → schedules another retry, stays submitting.
	l := newSubmitting()
	s, cmd := l.Update(runFoundMsg{id: 0, attempt: 0})
	if cmd == nil {
		t.Fatal("id==0 with attempts remaining should schedule a retry")
	}
	if s.(*launch).phase != phaseSubmitting {
		t.Fatalf("phase = %v, want phaseSubmitting during retry", s.(*launch).phase)
	}

	// Run not found, last attempt → gives up with an errMsg, resets to phaseInputs.
	l = newSubmitting()
	s, cmd = l.Update(runFoundMsg{id: 0, attempt: maxFindRunAttempts - 1})
	if cmd == nil {
		t.Fatal("exhausted retries should return a command")
	}
	if got := s.(*launch).phase; got != phaseInputs {
		t.Fatalf("phase = %v, want phaseInputs after exhaustion", got)
	}
	if _, ok := cmd().(errMsg); !ok {
		t.Fatalf("exhausted retries should yield errMsg, got %T", cmd())
	}

	// Run found → pushes the run detail screen (non-nil command).
	l = newSubmitting()
	_, cmd = l.Update(runFoundMsg{id: 42, attempt: 0})
	if cmd == nil {
		t.Fatal("a found run should push the run detail screen")
	}
}

// TestLaunchTextFieldReceivesKeys verifies that in the inputs phase, a string
// field's keystrokes reach its textinput (regression: handleKey used to consume
// all keys, so typing was impossible).
func TestLaunchTextFieldReceivesKeys(t *testing.T) {
	inputs := []gh.Input{{Name: "version", Type: gh.InputString}}
	l, _ := newLaunch(nil, gh.RepoRef{Owner: "o", Name: "r"}, gh.Workflow{ID: 1}, inputs, 20)
	l.phase = phaseInputs
	l.focusCurrent()
	// Type "1.2"
	for _, r := range "1.2" {
		l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := l.values()["version"]; got != "1.2" {
		t.Fatalf("typed value = %q, want %q", got, "1.2")
	}
}
