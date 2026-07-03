package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type rundetail struct {
	client   GHClient
	repo     gh.RepoRef
	id       int64
	detail   gh.RunDetail
	loaded   bool
	interval time.Duration
	// explainAvailable is stamped by the App on push: the explain service is
	// configured and enabled.
	explainAvailable bool
}

func newRunDetail(c GHClient, repo gh.RepoRef, id int64) (*rundetail, tea.Cmd) {
	d := &rundetail{client: c, repo: repo, id: id, interval: 4 * time.Second}
	return d, loadRunDetailCmd(c, repo, id)
}

func (d *rundetail) Title() string { return fmt.Sprintf("run #%d", d.id) }

func (d *rundetail) active() bool { return d.loaded && d.detail.Run.Active() }

// explainable reports whether the run's conclusion warrants an explanation.
func (d *rundetail) explainable() bool {
	return d.loaded && (d.detail.Conclusion == "failure" || d.detail.Conclusion == "timed_out")
}

// refresh reloads the run detail while the run is active; driven by the app's
// single ticker. Returns nil + a slow interval once the run is no longer active.
func (d *rundetail) refresh() (tea.Cmd, time.Duration) {
	if d.active() {
		return loadRunDetailCmd(d.client, d.repo, d.id), d.interval
	}
	return nil, 15 * time.Second
}

func (d *rundetail) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case runDetailLoadedMsg:
		if m.err != nil {
			return d, func() tea.Msg { return errMsg{err: m.err} }
		}
		d.detail = m.detail
		d.detail.Run.ID = d.id
		d.loaded = true
		return d, nil
	case actionDoneMsg:
		if m.err != nil {
			return d, func() tea.Msg { return errMsg{err: m.err} }
		}
		return d, loadRunDetailCmd(d.client, d.repo, d.id)
	case tea.KeyMsg:
		switch m.String() {
		case "l":
			lg, cmd := newLogs(d.client, d.repo, d.id, d.detail.Conclusion == "failure")
			return d, tea.Batch(func() tea.Msg { return pushMsg{screen: lg} }, cmd)
		case "r":
			return d, rerunCmd(d.client, d.repo, d.id, false)
		case "f":
			return d, rerunCmd(d.client, d.repo, d.id, true)
		case "x":
			return d, cancelCmd(d.client, d.repo, d.id)
		case "o":
			return d, openWebCmd(d.client, d.repo, d.id)
		case "e":
			if d.explainAvailable && d.explainable() {
				detail := d.detail
				return d, func() tea.Msg { return explainRunMsg{repo: d.repo, id: d.id, detail: detail} }
			}
		}
	}
	return d, nil
}

func (d *rundetail) View() string {
	if !d.loaded {
		return "Loading run…"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s run #%d — %s\n\n",
		statusIcon(d.detail.Status, d.detail.Conclusion), d.id, d.detail.Conclusion))
	for _, job := range d.detail.Jobs {
		b.WriteString(fmt.Sprintf("%s %s\n", statusIcon(job.Status, job.Conclusion), job.Name))
		for _, st := range job.Steps {
			b.WriteString(fmt.Sprintf("    %s %s\n", statusIcon(st.Status, st.Conclusion), st.Name))
		}
	}
	hints := "[l] logs  ·  r rerun  f rerun-failed  x cancel  o web"
	if d.explainAvailable && d.explainable() {
		hints += "  ·  e explain"
	}
	b.WriteString("\n" + hints)
	return b.String()
}
