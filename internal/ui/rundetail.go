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
}

func newRunDetail(c GHClient, repo gh.RepoRef, id int64) (*rundetail, tea.Cmd) {
	d := &rundetail{client: c, repo: repo, id: id, interval: 4 * time.Second}
	return d, tea.Batch(loadRunDetailCmd(c, repo, id), tickCmd(d.interval))
}

func (d *rundetail) Title() string { return fmt.Sprintf("run #%d", d.id) }

func (d *rundetail) active() bool { return d.loaded && d.detail.Run.Active() }

func (d *rundetail) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case runDetailLoadedMsg:
		if m.err != nil {
			return d, func() tea.Msg { return errMsg{err: m.err} }
		}
		d.detail = m.detail
		d.detail.Run.ID = d.id
		d.loaded = true
		if d.active() {
			return d, tickCmd(d.interval)
		}
		return d, nil
	case tickMsg:
		if d.active() {
			return d, loadRunDetailCmd(d.client, d.repo, d.id)
		}
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
		}
	}
	return d, nil
}

func (d *rundetail) View() string {
	if !d.loaded {
		return "Chargement du run…"
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
	b.WriteString("\n[l] logs  ·  r rerun  f rerun-failed  x cancel  o web")
	return b.String()
}
