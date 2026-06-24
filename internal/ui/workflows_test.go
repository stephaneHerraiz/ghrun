package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestWorkflowsLoadedPopulatesList(t *testing.T) {
	w, _ := newWorkflows(nil, gh.RepoRef{Owner: "o", Name: "r"})
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
