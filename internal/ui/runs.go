package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type runs struct {
	client     GHClient
	repo       gh.RepoRef
	limit      int
	items      []gh.Run
	listScroll // cursor + vertical scroll window
	loading    bool
	interval   time.Duration
}

func newRuns(c GHClient, repo gh.RepoRef, limit, pageSize int) (*runs, tea.Cmd) {
	if limit <= 0 {
		limit = 30
	}
	r := &runs{
		client:     c,
		repo:       repo,
		limit:      limit,
		listScroll: listScroll{pageSize: pageSize},
		loading:    true,
		interval:   4 * time.Second,
	}
	return r, loadRunsCmd(c, repo, limit)
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

// refresh reloads runs while any is active; the app shell drives this on its
// single ticker. Returns nil + a slow interval when nothing is active.
func (r *runs) refresh() (tea.Cmd, time.Duration) {
	if r.anyActive() {
		return loadRunsCmd(r.client, r.repo, r.limit), r.interval
	}
	return nil, 15 * time.Second
}

func (r *runs) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case runsLoadedMsg:
		r.loading = false
		if m.err != nil {
			return r, func() tea.Msg { return errMsg{err: m.err} }
		}
		r.items = m.runs
		r.clampCursor(len(r.items)) // keep the cursor in range after a reload
		return r, nil
	case actionDoneMsg:
		if m.err != nil {
			return r, func() tea.Msg { return errMsg{err: m.err} }
		}
		return r, loadRunsCmd(r.client, r.repo, r.limit)
	case tea.MouseMsg:
		r.handleWheel(m, len(r.items))
		return r, nil
	case tea.KeyMsg:
		return r.handleKey(m)
	}
	return r, nil
}

func (r *runs) handleKey(m tea.KeyMsg) (Screen, tea.Cmd) {
	switch m.String() {
	case "up", "k":
		r.up()
	case "down", "j":
		r.down(len(r.items))
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
	r.ensureVisible(len(r.items))
	return r, nil
}

func (r *runs) View() string {
	if r.loading && len(r.items) == 0 {
		return "Loading runs…"
	}
	if len(r.items) == 0 {
		return "No runs."
	}
	lines := make([]string, len(r.items))
	for i, run := range r.items {
		cursor := "  "
		if i == r.cursor {
			cursor = "▸ "
		}
		lines[i] = fmt.Sprintf("%s%s %-18s %-16s %s",
			cursor, statusIcon(run.Status, run.Conclusion), run.WorkflowName, run.HeadBranch, run.Title)
	}
	var b strings.Builder
	b.WriteString(r.render(lines))
	b.WriteString("\n\n[Enter] detail  ·  r rerun  f rerun-failed  x cancel  o web  g refresh")
	return b.String()
}
