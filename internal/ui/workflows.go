package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type workflows struct {
	client  GHClient
	repo    gh.RepoRef
	items   []gh.Workflow
	cursor  int
	loading bool
}

func newWorkflows(c GHClient, repo gh.RepoRef) (*workflows, tea.Cmd) {
	w := &workflows{client: c, repo: repo, loading: true}
	return w, loadWorkflowsCmd(c, repo)
}

func (w *workflows) Title() string { return "Workflows" }

func (w *workflows) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case workflowsLoadedMsg:
		w.loading = false
		if m.err != nil {
			return w, func() tea.Msg { return errMsg{err: m.err} }
		}
		w.items = m.workflows
		return w, nil
	case tea.KeyMsg:
		switch m.String() {
		case "up", "k":
			if w.cursor > 0 {
				w.cursor--
			}
		case "down", "j":
			if w.cursor < len(w.items)-1 {
				w.cursor++
			}
		case "enter":
			if w.cursor < len(w.items) {
				wf := w.items[w.cursor]
				return w, loadInputsCmd(w.client, w.repo, wf)
			}
		}
	case inputsLoadedMsg:
		// Build the launch screen from the loaded inputs and push it.
		if m.err != nil {
			return w, func() tea.Msg { return errMsg{err: m.err} }
		}
		var wf gh.Workflow
		for _, it := range w.items {
			if it.ID == m.workflowID {
				wf = it
				break
			}
		}
		lc, cmd := newLaunch(w.client, w.repo, wf, m.inputs)
		return w, tea.Batch(func() tea.Msg { return pushMsg{screen: lc} }, cmd)
	}
	return w, nil
}

func (w *workflows) View() string {
	if w.loading {
		return "Chargement des workflows…"
	}
	if len(w.items) == 0 {
		return "Aucun workflow."
	}
	var b strings.Builder
	for i, wf := range w.items {
		cursor := "  "
		if i == w.cursor {
			cursor = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, wf.Name))
	}
	b.WriteString("\n[Enter] configurer le lancement")
	return b.String()
}
