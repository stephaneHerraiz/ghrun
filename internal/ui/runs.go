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
	width      int // terminal width, for full-width column layout (0 = unknown)
	listScroll     // cursor + vertical scroll window
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
	case tea.WindowSizeMsg:
		r.width = m.Width
		return r, nil
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

// runsLayout holds the per-column display widths for the runs table.
type runsLayout struct{ wf, branch, title int }

// runsLayoutFor sizes the workflow, branch, and title columns to fill width w.
// The title column absorbs most of the flexible space; workflow and branch are
// capped so they don't dominate a wide terminal. The scrollbar gutter is
// reserved only when scrollbar is true, so the row fills the full width when no
// scrollbar is drawn. A width of 0 (not yet known) falls back to fixed widths;
// a known-but-narrow width is clamped to the column floors.
func runsLayoutFor(w int, scrollbar bool) runsLayout {
	if w <= 0 {
		return runsLayout{wf: 18, branch: 16, title: 40}
	}
	gutter := 0
	if scrollbar {
		gutter = 3
	}
	// Reserved: cursor (2) + icon (1) + three single-space gaps + the gutter.
	flex := max(10+8+10, w-(2+1+3+gutter))
	wf := min(28, max(10, flex*30/100))
	branch := min(24, max(8, flex*25/100))
	return runsLayout{wf: wf, branch: branch, title: flex - wf - branch}
}

func (r *runs) View() string {
	if r.loading && len(r.items) == 0 {
		return "Loading runs…"
	}
	if len(r.items) == 0 {
		return "No runs."
	}
	L := runsLayoutFor(r.width, len(r.items) > r.page())
	lines := make([]string, len(r.items))
	for i, run := range r.items {
		cursor := "  "
		if i == r.cursor {
			cursor = "▸ "
		}
		lines[i] = fmt.Sprintf("%s%s %s %s %s",
			cursor, padCell(statusIcon(run.Status, run.Conclusion), 1),
			padCell(run.WorkflowName, L.wf), padCell(run.HeadBranch, L.branch),
			padCell(run.Title, L.title))
	}
	var b strings.Builder
	b.WriteString(r.render(lines))
	b.WriteString("\n\n[Enter] detail  ·  r rerun  f rerun-failed  x cancel  o web  g refresh")
	return b.String()
}
