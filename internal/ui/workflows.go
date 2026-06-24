package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type workflows struct {
	client     GHClient
	repo       gh.RepoRef
	items      []gh.Workflow
	listScroll // cursor + vertical scroll window
	loading    bool
}

func newWorkflows(c GHClient, repo gh.RepoRef, pageSize int) (*workflows, tea.Cmd) {
	w := &workflows{client: c, repo: repo, listScroll: listScroll{pageSize: pageSize}, loading: true}
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
		w.clampCursor(len(w.items))
		return w, nil
	case tea.MouseMsg:
		w.handleWheel(m, len(w.items))
		return w, nil
	case tea.KeyMsg:
		switch m.String() {
		case "up", "k":
			w.up()
			w.ensureVisible(len(w.items))
		case "down", "j":
			w.down(len(w.items))
			w.ensureVisible(len(w.items))
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
		lc, cmd := newLaunch(w.client, w.repo, wf, m.inputs, w.pageSize)
		return w, tea.Batch(func() tea.Msg { return pushMsg{screen: lc} }, cmd)
	}
	return w, nil
}

func (w *workflows) View() string {
	if w.loading {
		return "Loading workflows…"
	}
	if len(w.items) == 0 {
		return "No workflows."
	}
	lines := make([]string, len(w.items))
	for i, wf := range w.items {
		cursor := "  "
		if i == w.cursor {
			cursor = "▸ "
		}
		lines[i] = fmt.Sprintf("%s%s", cursor, wf.Name)
	}
	var b strings.Builder
	b.WriteString(w.render(lines))
	b.WriteString("\n\n[Enter] configure launch")
	return b.String()
}
