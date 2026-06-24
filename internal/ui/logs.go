package ui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type logs struct {
	client     GHClient
	repo       gh.RepoRef
	id         int64
	failedOnly bool
	content    string
	errText    string
	loading    bool
	vp         viewport.Model
	ready      bool
}

func newLogs(c GHClient, repo gh.RepoRef, id int64, failedOnly bool) (*logs, tea.Cmd) {
	l := &logs{client: c, repo: repo, id: id, failedOnly: failedOnly, loading: true}
	return l, loadLogsCmd(c, repo, id, failedOnly)
}

func (l *logs) Title() string { return "logs" }

func (l *logs) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case logsLoadedMsg:
		l.loading = false
		if m.err != nil {
			l.errText = m.err.Error()
			return l, nil
		}
		l.content = m.text
		if l.ready {
			l.vp.SetContent(l.content)
		}
		return l, nil
	case tea.WindowSizeMsg:
		if !l.ready {
			l.vp = viewport.New(m.Width, max(3, m.Height-6))
			l.ready = true
		} else {
			l.vp.Width = m.Width
			l.vp.Height = max(3, m.Height-6)
		}
		l.vp.SetContent(l.content)
		return l, nil
	case tea.KeyMsg:
		var cmd tea.Cmd
		l.vp, cmd = l.vp.Update(msg)
		return l, cmd
	case tea.MouseMsg:
		var cmd tea.Cmd
		l.vp, cmd = l.vp.Update(msg)
		return l, cmd
	}
	return l, nil
}

func (l *logs) View() string {
	if l.loading {
		return "Loading logs…"
	}
	if l.errText != "" {
		return errStyle.Render("⚠ " + l.errText)
	}
	if !l.ready {
		return l.content
	}
	return l.vp.View()
}
