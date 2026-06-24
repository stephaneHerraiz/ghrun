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

func TestRunsRefresh(t *testing.T) {
	r, _ := newRuns(nil, gh.RepoRef{Owner: "o", Name: "r"}, 30)
	// idle (no items) → no reload, slow interval
	if cmd, d := r.refresh(); cmd != nil || d < 10*time.Second {
		t.Fatalf("idle refresh = (cmd!=nil:%v, %v), want (nil, slow >=10s)", cmd != nil, d)
	}
	// active → reload + fast interval
	r.items = []gh.Run{{ID: 1, Status: "in_progress"}}
	if cmd, d := r.refresh(); cmd == nil || d > 10*time.Second {
		t.Fatalf("active refresh = (cmd!=nil:%v, %v), want (non-nil, fast)", cmd != nil, d)
	}
}
