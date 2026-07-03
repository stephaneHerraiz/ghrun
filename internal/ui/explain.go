package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// explainPhase tracks the screen's async pipeline: log → local RAG → claude.
type explainPhase int

const (
	phaseLoadingLog explainPhase = iota
	phaseSearching
	phaseAsking
	phaseDone
	phaseFailed
)

// explainScreen shows the explanation of a failed run (spec: écran Explanation).
type explainScreen struct {
	client GHClient
	svc    ExplainService
	repo   gh.RepoRef
	id     int64
	detail gh.RunDetail

	phase    explainPhase
	logText  string
	content  string // explanation currently shown
	badge    string // source badge (🧠 local / ✨ claude / best guess)
	warnText string // non-fatal warning (Ollama down, claude fallback…)
	errText  string
	guess    string // best sub-threshold local match, shown if claude fails
	guessSim float32

	vp    viewport.Model
	ready bool
}

func newExplain(c GHClient, svc ExplainService, repo gh.RepoRef, id int64, detail gh.RunDetail) (*explainScreen, tea.Cmd) {
	e := &explainScreen{client: c, svc: svc, repo: repo, id: id, detail: detail, phase: phaseLoadingLog}
	return e, loadExplainLogCmd(c, repo, id)
}

func (e *explainScreen) Title() string { return "explain" }

// request builds the claude request from the run context and the full log.
func (e *explainScreen) request() explain.ExplainRequest {
	return explain.ExplainRequest{
		Log:         e.logText,
		Repo:        e.repo.String(),
		Workflow:    e.detail.WorkflowName,
		FailedSteps: failedSteps(e.detail),
	}
}

// setContent stores and (re)wraps the explanation into the viewport.
func (e *explainScreen) setContent(text string) {
	e.content = text
	if e.ready {
		e.vp.SetContent(lipgloss.NewStyle().Width(e.vp.Width).Render(text))
	}
}

func (e *explainScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case explainLogLoadedMsg:
		if m.err != nil {
			e.phase = phaseFailed
			e.errText = m.err.Error()
			return e, nil
		}
		e.logText = m.text
		e.phase = phaseSearching
		return e, resolveLocalCmd(e.svc, e.logText)

	case explainLocalMsg:
		if e.phase != phaseSearching {
			return e, nil // stale result from a superseded search
		}
		if m.err != nil {
			e.warnText = "knowledge base unavailable: " + m.err.Error()
			e.phase = phaseAsking
			return e, askClaudeCmd(e.svc, e.request())
		}
		if m.res.RAGDisabled {
			e.warnText = "knowledge base disabled (Ollama unreachable)"
		}
		if m.res.Found {
			e.phase = phaseDone
			e.badge = localBadge(m.res)
			e.setContent(m.res.Explanation)
			return e, nil
		}
		e.guess, e.guessSim = m.res.Explanation, m.res.Similarity
		e.phase = phaseAsking
		return e, askClaudeCmd(e.svc, e.request())

	case explainClaudeMsg:
		if e.phase != phaseAsking {
			return e, nil // stale result from a superseded request
		}
		if m.err != nil {
			if e.guess != "" {
				e.phase = phaseDone
				e.badge = fmt.Sprintf("🧠 best guess · %.0f%%", e.guessSim*100)
				e.warnText = "claude unavailable: " + m.err.Error()
				e.setContent(e.guess)
				return e, nil
			}
			e.phase = phaseFailed
			e.errText = m.err.Error() + " — press l to open the raw logs"
			return e, nil
		}
		e.phase = phaseDone
		e.badge = "✨ " + m.res.Source
		e.setContent(m.res.Explanation)
		return e, nil

	case tea.WindowSizeMsg:
		if !e.ready {
			e.vp = viewport.New(m.Width, max(3, m.Height-8))
			e.ready = true
		} else {
			e.vp.Width = m.Width
			e.vp.Height = max(3, m.Height-8)
		}
		e.setContent(e.content)
		return e, nil

	case tea.KeyMsg:
		switch m.String() {
		case "r":
			if e.logText != "" && (e.phase == phaseDone || e.phase == phaseFailed) {
				e.phase = phaseAsking
				e.badge = ""
				e.warnText = ""
				return e, askClaudeCmd(e.svc, e.request())
			}
			return e, nil
		case "l":
			lg, cmd := newLogs(e.client, e.repo, e.id, true)
			return e, tea.Batch(func() tea.Msg { return pushMsg{screen: lg} }, cmd)
		}
		var cmd tea.Cmd
		e.vp, cmd = e.vp.Update(msg)
		return e, cmd

	case tea.MouseMsg:
		var cmd tea.Cmd
		e.vp, cmd = e.vp.Update(msg)
		return e, cmd
	}
	return e, nil
}

func (e *explainScreen) View() string {
	var b strings.Builder
	header := fmt.Sprintf("run #%d — %s", e.id, e.repo.String())
	if e.badge != "" {
		header += "   " + e.badge
	}
	b.WriteString(header + "\n")
	if e.warnText != "" {
		b.WriteString(footerStyle.Render("⚠ "+e.warnText) + "\n")
	}
	b.WriteString("\n")
	switch e.phase {
	case phaseLoadingLog:
		b.WriteString("Fetching failed log…")
	case phaseSearching:
		b.WriteString("Searching local knowledge…")
	case phaseAsking:
		b.WriteString("Asking claude… (may take a minute)")
	case phaseFailed:
		b.WriteString(errStyle.Render("⚠ " + e.errText))
	case phaseDone:
		if e.ready {
			b.WriteString(e.vp.View())
		} else {
			b.WriteString(e.content)
		}
	}
	b.WriteString("\n\n" + footerStyle.Render("r regenerate · l raw logs"))
	return b.String()
}

// localBadge renders the knowledge-base hit badge. %.0f rounds instead of
// truncating (float32 0.92*100 can land just below 92).
func localBadge(res explain.LocalResult) string {
	if res.Exact {
		return "🧠 local · exact"
	}
	return fmt.Sprintf("🧠 local · %.0f%%", res.Similarity*100)
}

// failedSteps lists "job / step" pairs whose conclusion is failure.
func failedSteps(d gh.RunDetail) []string {
	var out []string
	for _, j := range d.Jobs {
		for _, s := range j.Steps {
			if s.Conclusion == "failure" {
				out = append(out, j.Name+" / "+s.Name)
			}
		}
	}
	return out
}
