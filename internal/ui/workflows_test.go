package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestWorkflowsPaginatesAndMouseScrolls(t *testing.T) {
	w, _ := newWorkflows(nil, gh.RepoRef{Owner: "o", Name: "r"}, 3) // page size 3
	items := make([]gh.Workflow, 6)
	for i := range items {
		items[i] = gh.Workflow{ID: int64(i), Name: fmt.Sprintf("wf%d", i)}
	}
	s, _ := w.Update(workflowsLoadedMsg{workflows: items})
	w = s.(*workflows)

	v := w.View()
	if strings.Count(v, "wf") != 3 {
		t.Fatalf("expected 3 workflow rows shown (page size 3), got %d:\n%s", strings.Count(v, "wf"), v)
	}
	if !strings.Contains(v, scrollThumb) {
		t.Errorf("overflowing workflows list should render a scrollbar; got:\n%s", v)
	}

	// Mouse wheel down moves the cursor and scrolls the window.
	for range 4 {
		s, _ = w.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
		w = s.(*workflows)
	}
	if w.cursor != 4 || w.offset != 2 {
		t.Fatalf("cursor/offset = %d/%d after 4 wheel-down, want 4/2", w.cursor, w.offset)
	}
}

func TestWorkflowsLoadedPopulatesList(t *testing.T) {
	w, _ := newWorkflows(nil, gh.RepoRef{Owner: "o", Name: "r"}, 20)
	s, _ := w.Update(workflowsLoadedMsg{workflows: []gh.Workflow{
		{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"},
		{ID: 2, Name: "Deploy", Path: ".github/workflows/deploy.yml"},
	}})
	wf := s.(*workflows)
	if len(wf.items) != 2 || wf.items[1].Name != "Deploy" {
		t.Fatalf("items = %+v", wf.items)
	}
	if wf.Title() != "Workflows" {
		t.Errorf("title = %q", wf.Title())
	}
	// Down + enter selects Deploy and asks to load its inputs.
	wf.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := wf.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a command to load inputs")
	}
}
