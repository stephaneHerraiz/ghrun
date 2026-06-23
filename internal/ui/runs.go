package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type runs struct {
	client   GHClient
	repo     gh.RepoRef
	limit    int
	items    []gh.Run
	cursor   int
	loading  bool
	interval time.Duration
}

func newRuns(c GHClient, repo gh.RepoRef, limit int) (*runs, tea.Cmd) {
	if limit <= 0 {
		limit = 30
	}
	r := &runs{client: c, repo: repo, limit: limit, loading: true, interval: 4 * time.Second}
	return r, tea.Batch(loadRunsCmd(c, repo, limit), tickCmd(r.interval))
}

func (r *runs) Title() string { return "Runs" }

func (r *runs) anyActive() bool {
	for _, run := range r.items {
		if run.Active() {
			return true
		}
	}
	return false
}

func (r *runs) selected() (gh.Run, bool) {
	if r.cursor < len(r.items) {
		return r.items[r.cursor], true
	}
	return gh.Run{}, false
}

func (r *runs) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case runsLoadedMsg:
		r.loading = false
		if m.err != nil {
			return r, func() tea.Msg { return errMsg{err: m.err} }
		}
		r.items = m.runs
		if r.cursor >= len(r.items) {
			r.cursor = 0
		}
		// Do not start a tick here: a single self-sustaining tick chain is
		// born in newRuns. Emitting one here too would double-poll.
		return r, nil
	case tickMsg:
		// Single self-sustaining ticker: reload only while active, slow down
		// (but keep ticking) when idle so polling resumes if a run reactivates.
		next := r.interval
		var reload tea.Cmd
		if r.anyActive() {
			reload = loadRunsCmd(r.client, r.repo, r.limit)
		} else {
			next = 15 * time.Second
		}
		return r, tea.Batch(reload, tickCmd(next))
	case actionDoneMsg:
		if m.err != nil {
			return r, func() tea.Msg { return errMsg{err: m.err} }
		}
		return r, loadRunsCmd(r.client, r.repo, r.limit)
	case tea.KeyMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func (r *runs) handleKey(m tea.KeyMsg) (Screen, tea.Cmd) {
	switch m.String() {
	case "up", "k":
		if r.cursor > 0 {
			r.cursor--
		}
	case "down", "j":
		if r.cursor < len(r.items)-1 {
			r.cursor++
		}
	case "g":
		return r, loadRunsCmd(r.client, r.repo, r.limit)
	case "enter":
		if run, ok := r.selected(); ok {
			rd, cmd := newRunDetail(r.client, r.repo, run.ID)
			return r, tea.Batch(func() tea.Msg { return pushMsg{screen: rd} }, cmd)
		}
	case "r":
		if run, ok := r.selected(); ok {
			return r, rerunCmd(r.client, r.repo, run.ID, false)
		}
	case "f":
		if run, ok := r.selected(); ok {
			return r, rerunCmd(r.client, r.repo, run.ID, true)
		}
	case "x":
		if run, ok := r.selected(); ok {
			return r, cancelCmd(r.client, r.repo, run.ID)
		}
	case "o":
		if run, ok := r.selected(); ok {
			return r, openWebCmd(r.client, r.repo, run.ID)
		}
	}
	return r, nil
}

func (r *runs) View() string {
	if r.loading && len(r.items) == 0 {
		return "Chargement des runs…"
	}
	if len(r.items) == 0 {
		return "Aucun run."
	}
	var b strings.Builder
	for i, run := range r.items {
		cursor := "  "
		if i == r.cursor {
			cursor = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s%s %-18s %-16s %s\n",
			cursor, statusIcon(run.Status, run.Conclusion), run.WorkflowName, run.HeadBranch, run.Title))
	}
	b.WriteString("\n[Enter] détail  ·  r rerun  f rerun-failed  x cancel  o web  g refresh")
	return b.String()
}
