package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestRunsLoadedAndEnter(t *testing.T) {
	r, _ := newRuns(nil, gh.RepoRef{Owner: "o", Name: "r"}, 30)
	s, _ := r.Update(runsLoadedMsg{runs: []gh.Run{
		{ID: 10, WorkflowName: "CI", Status: "in_progress", HeadBranch: "main"},
		{ID: 9, WorkflowName: "Deploy", Status: "completed", Conclusion: "success", HeadBranch: "main"},
	}})
	rs := s.(*runs)
	if len(rs.items) != 2 {
		t.Fatalf("items = %d", len(rs.items))
	}
	if rs.Title() != "Runs" {
		t.Errorf("title = %q", rs.Title())
	}
	_, cmd := rs.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should push run detail")
	}
}

func TestRunsActiveTickReloadsAndResustains(t *testing.T) {
	r, _ := newRuns(nil, gh.RepoRef{Owner: "o", Name: "r"}, 30)
	// Loading runs must NOT emit its own tick — the ticker self-sustains from
	// newRuns, so loaded returning a tick too would double-poll.
	s, loadedCmd := r.Update(runsLoadedMsg{runs: []gh.Run{{ID: 1, Status: "in_progress"}}})
	if loadedCmd != nil {
		t.Fatal("runsLoadedMsg must not emit a tick (would double-poll)")
	}
	rs := s.(*runs)
	// A tick while runs are active keeps the single chain alive: it reloads and
	// re-ticks, so the returned command is non-nil.
	_, tickCmdResult := rs.Update(tickMsg(time.Time{}))
	if tickCmdResult == nil {
		t.Fatal("a tick with active runs should reload + re-tick")
	}
}
