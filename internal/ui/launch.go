package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type launchPhase int

const (
	phaseBranch launchPhase = iota
	phaseInputs
	phaseSubmitting
)

// field holds editing state for one input.
type field struct {
	in       gh.Input
	text     textinput.Model // for string/number
	choiceIx int             // for choice
	boolVal  bool            // for boolean
}

type launch struct {
	client     GHClient
	repo       gh.RepoRef
	wf         gh.Workflow
	phase      launchPhase
	branches   []string
	branchIx   int
	fields     []field
	cursor     int
	missing    []string
	dispatched time.Time
}

func newLaunch(c GHClient, repo gh.RepoRef, wf gh.Workflow, inputs []gh.Input) (*launch, tea.Cmd) {
	l := &launch{client: c, repo: repo, wf: wf, phase: phaseBranch}
	for _, in := range inputs {
		f := field{in: in}
		switch in.Type {
		case gh.InputBoolean:
			f.boolVal = in.Default == "true"
		case gh.InputChoice:
			f.choiceIx = 0 // defaults to first option
			if in.Default != "" {
				for i, o := range in.Options {
					if o == in.Default {
						f.choiceIx = i
					}
				}
			}
		default:
			ti := textinput.New()
			ti.SetValue(in.Default)
			f.text = ti
		}
		l.fields = append(l.fields, f)
	}
	return l, loadBranchesCmd(c, repo)
}

func (l *launch) Title() string { return "launch: " + l.wf.Name }

// values returns the current input values as strings.
func (l *launch) values() map[string]string {
	vals := map[string]string{}
	for _, f := range l.fields {
		switch f.in.Type {
		case gh.InputBoolean:
			vals[f.in.Name] = fmt.Sprintf("%t", f.boolVal)
		case gh.InputChoice:
			if f.choiceIx < len(f.in.Options) {
				vals[f.in.Name] = f.in.Options[f.choiceIx]
			}
		default:
			vals[f.in.Name] = f.text.Value()
		}
	}
	return vals
}

// validate returns names of required inputs that are empty.
func (l *launch) validate() []string {
	var missing []string
	vals := l.values()
	for _, f := range l.fields {
		if f.in.Required && strings.TrimSpace(vals[f.in.Name]) == "" {
			missing = append(missing, f.in.Name)
		}
	}
	return missing
}

func (l *launch) currentBranch() string {
	if l.branchIx < len(l.branches) {
		return l.branches[l.branchIx]
	}
	return ""
}

func (l *launch) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case branchesLoadedMsg:
		if m.err != nil {
			return l, func() tea.Msg { return errMsg{err: m.err} }
		}
		l.branches = m.branches
		return l, nil
	case dispatchedMsg:
		if m.err != nil {
			l.phase = phaseInputs
			return l, func() tea.Msg { return errMsg{err: m.err} }
		}
		l.dispatched = m.since
		return l, l.findRunCmd(0)
	case runFoundMsg:
		if m.err != nil {
			return l, func() tea.Msg { return errMsg{err: m.err} }
		}
		if m.id == 0 {
			return l, l.findRunCmd(1) // retry chain handled below
		}
		rd, cmd := newRunDetail(l.client, l.repo, m.id)
		return l, tea.Batch(func() tea.Msg { return pushMsg{screen: rd} }, cmd)
	case tea.KeyMsg:
		return l.handleKey(m)
	}
	// delegate to active text field
	if l.phase == phaseInputs && l.cursor < len(l.fields) {
		f := &l.fields[l.cursor]
		if f.in.Type != gh.InputBoolean && f.in.Type != gh.InputChoice {
			var cmd tea.Cmd
			f.text, cmd = f.text.Update(msg)
			return l, cmd
		}
	}
	return l, nil
}

func (l *launch) handleKey(m tea.KeyMsg) (Screen, tea.Cmd) {
	switch l.phase {
	case phaseBranch:
		switch m.String() {
		case "up", "k":
			if l.branchIx > 0 {
				l.branchIx--
			}
		case "down", "j":
			if l.branchIx < len(l.branches)-1 {
				l.branchIx++
			}
		case "enter":
			l.phase = phaseInputs
			l.focusCurrent()
		}
		return l, nil
	case phaseInputs:
		switch m.String() {
		case "up":
			if l.cursor > 0 {
				l.cursor--
				l.focusCurrent()
			}
		case "down":
			if l.cursor < len(l.fields)-1 {
				l.cursor++
				l.focusCurrent()
			}
		case "left", "right":
			l.adjustChoice(&l.fields[l.cursor], m.String() == "right")
		case "ctrl+s":
			return l.submit()
		}
	}
	return l, nil
}

func (l *launch) focusCurrent() {
	for i := range l.fields {
		if i == l.cursor && l.fields[i].in.Type != gh.InputBoolean && l.fields[i].in.Type != gh.InputChoice {
			l.fields[i].text.Focus()
		} else {
			l.fields[i].text.Blur()
		}
	}
}

func (l *launch) adjustChoice(f *field, forward bool) {
	switch f.in.Type {
	case gh.InputBoolean:
		f.boolVal = !f.boolVal
	case gh.InputChoice:
		if len(f.in.Options) == 0 {
			return
		}
		if forward {
			f.choiceIx = (f.choiceIx + 1) % len(f.in.Options)
		} else {
			f.choiceIx = (f.choiceIx - 1 + len(f.in.Options)) % len(f.in.Options)
		}
	}
}

func (l *launch) submit() (Screen, tea.Cmd) {
	l.missing = l.validate()
	if len(l.missing) > 0 {
		return l, nil
	}
	l.phase = phaseSubmitting
	repo, wf, ref, vals := l.repo, l.wf, l.currentBranch(), l.values()
	c := l.client
	return l, func() tea.Msg {
		err := c.DispatchWorkflow(repo, wf.ID, ref, vals)
		return dispatchedMsg{since: time.Now().Add(-2 * time.Second), err: err}
	}
}

// findRunCmd polls for the dispatched run with bounded retries.
func (l *launch) findRunCmd(attempt int) tea.Cmd {
	repo, wfID, since := l.repo, l.wf.ID, l.dispatched
	c := l.client
	return func() tea.Msg {
		// brief backoff before each lookup (run appears with a delay)
		time.Sleep(time.Duration(1+attempt) * time.Second)
		id, err := c.FindRunSince(repo, wfID, since)
		if err == nil && id == 0 && attempt < 5 {
			id2, err2 := c.FindRunSince(repo, wfID, since)
			return runFoundMsg{id: id2, err: err2}
		}
		return runFoundMsg{id: id, err: err}
	}
}

func (l *launch) View() string {
	var b strings.Builder
	switch l.phase {
	case phaseBranch:
		b.WriteString("Choisir la branche (ref) :\n\n")
		for i, br := range l.branches {
			cursor := "  "
			if i == l.branchIx {
				cursor = "▸ "
			}
			b.WriteString(cursor + br + "\n")
		}
		if len(l.branches) == 0 {
			b.WriteString("(chargement des branches…)\n")
		}
		b.WriteString("\n[Enter] continuer")
	case phaseInputs:
		b.WriteString(fmt.Sprintf("Branche: %s\n\n", l.currentBranch()))
		for i, f := range l.fields {
			cursor := "  "
			if i == l.cursor {
				cursor = "▸ "
			}
			req := ""
			if f.in.Required {
				req = "*"
			}
			b.WriteString(fmt.Sprintf("%s%s%s: %s\n", cursor, f.in.Name, req, l.renderField(f)))
		}
		if len(l.missing) > 0 {
			b.WriteString(errStyle.Render("\nChamps requis manquants: "+strings.Join(l.missing, ", ")) + "\n")
		}
		b.WriteString("\n[ctrl+s] lancer  ·  ←/→ change choix/booléen")
	case phaseSubmitting:
		b.WriteString("Lancement en cours, recherche du run…")
	}
	return b.String()
}

func (l *launch) renderField(f field) string {
	switch f.in.Type {
	case gh.InputBoolean:
		return fmt.Sprintf("%t", f.boolVal)
	case gh.InputChoice:
		if f.choiceIx < len(f.in.Options) {
			return "< " + f.in.Options[f.choiceIx] + " >"
		}
		return "—"
	default:
		return f.text.View()
	}
}
