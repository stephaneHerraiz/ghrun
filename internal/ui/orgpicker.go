package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// orgpicker lets the user choose a default org/account on first launch.
type orgpicker struct {
	client  GHClient
	items   []string
	cursor  int
	loading bool
	errText string
}

func newOrgPicker(c GHClient) (*orgpicker, tea.Cmd) {
	p := &orgpicker{client: c, loading: true}
	return p, p.initCmd()
}

func (p *orgpicker) initCmd() tea.Cmd { return loadNamespacesCmd(p.client) }

func (p *orgpicker) Title() string { return "Choose an organization" }

func (p *orgpicker) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case namespacesLoadedMsg:
		p.loading = false
		if m.err != nil {
			p.errText = m.err.Error()
			return p, nil
		}
		p.errText = ""
		p.items = m.names
		if p.cursor >= len(p.items) {
			p.cursor = 0
		}
		return p, nil
	case tea.KeyMsg:
		switch m.String() {
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "down", "j":
			if p.cursor < len(p.items)-1 {
				p.cursor++
			}
		case "g":
			p.loading = true
			return p, p.initCmd()
		case "enter":
			if p.cursor < len(p.items) {
				org := p.items[p.cursor]
				return p, func() tea.Msg { return orgSelectedMsg{org: org} }
			}
		}
	}
	return p, nil
}

func (p *orgpicker) View() string {
	if p.loading {
		return "Loading organizations…"
	}
	if p.errText != "" {
		return errStyle.Render("⚠ "+p.errText) + "\n\n[g] retry  ·  q quit"
	}
	if len(p.items) == 0 {
		return "No organization found.\n\n[g] retry  ·  q quit"
	}
	var b strings.Builder
	b.WriteString("Choose the default organization or account:\n\n")
	for i, name := range p.items {
		cursor := "  "
		if i == p.cursor {
			cursor = "▸ "
		}
		b.WriteString(cursor + name + "\n")
	}
	b.WriteString("\n[Enter] choose  ·  [g] refresh")
	return b.String()
}
