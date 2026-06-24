package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestRunsPaginatesAndMouseScrolls(t *testing.T) {
	r, _ := newRuns(nil, gh.RepoRef{Owner: "o", Name: "r"}, 30, 3) // page size 3
	items := make([]gh.Run, 6)
	for i := range items {
		items[i] = gh.Run{ID: int64(i), WorkflowName: fmt.Sprintf("wf%d", i), Status: "completed", Conclusion: "success"}
	}
	s, _ := r.Update(runsLoadedMsg{runs: items})
	r = s.(*runs)

	v := r.View()
	if strings.Count(v, "wf") != 3 {
		t.Fatalf("expected 3 run rows shown (page size 3), got %d:\n%s", strings.Count(v, "wf"), v)
	}
	if !strings.Contains(v, scrollThumb) {
		t.Errorf("overflowing runs list should render a scrollbar; got:\n%s", v)
	}

	// Mouse wheel down moves the cursor and scrolls the window.
	for range 4 {
		s, _ = r.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
		r = s.(*runs)
	}
	if r.cursor != 4 || r.offset != 2 {
		t.Fatalf("cursor/offset = %d/%d after 4 wheel-down, want 4/2", r.cursor, r.offset)
	}
}

func TestRunsLoadedAndEnter(t *testing.T) {
	r, _ := newRuns(nil, gh.RepoRef{Owner: "o", Name: "r"}, 30, 20)
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

func TestRunsViewFillsTerminalWidth(t *testing.T) {
	r, _ := newRuns(nil, gh.RepoRef{Owner: "o", Name: "r"}, 30, 20)
	s, _ := r.Update(runsLoadedMsg{runs: []gh.Run{
		{ID: 1, WorkflowName: "CI", HeadBranch: "main", Title: "build", Status: "completed", Conclusion: "success"},
	}})
	r = s.(*runs)
	s, _ = r.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	r = s.(*runs)

	v := r.View()
	// A single run does not overflow the page, so no scrollbar is drawn and the
	// row must fill the terminal width exactly (140).
	for line := range strings.SplitSeq(v, "\n") {
		if strings.Contains(line, "CI") {
			if w := lipgloss.Width(line); w != 140 {
				t.Fatalf("run row width = %d, want exactly 140 (full terminal width)\nline=%q", w, line)
			}
			return
		}
	}
	t.Fatalf("did not find the run row in:\n%s", v)
}

func TestRunsRefresh(t *testing.T) {
	r, _ := newRuns(nil, gh.RepoRef{Owner: "o", Name: "r"}, 30, 20)
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
