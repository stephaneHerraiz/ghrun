package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// inputCapturer is implemented by screens that consume raw keystrokes (e.g. a
// filter box), so the app shell must not treat those keys as global shortcuts.
type inputCapturer interface{ capturingInput() bool }

// App is the root Bubbletea model: it owns the screen stack and global chrome.
type App struct {
	client   GHClient
	cfg      config.Config
	stack    []Screen
	repo     *gh.RepoRef // current repo context (nil at dashboard)
	width    int
	height   int
	errText  string
	showHelp bool
}

// NewApp builds the root model with the dashboard seeded as the initial screen.
func NewApp(c GHClient, cfg config.Config) App {
	a := App{client: c, cfg: cfg}
	dash, _ := newDashboard(c, cfg)
	a.stack = []Screen{dash}
	return a
}

// Init kicks off the dashboard's initial load and tick.
func (a App) Init() tea.Cmd {
	if d, ok := a.top().(*dashboard); ok {
		return d.initCmd()
	}
	return nil
}

// push appends a screen to the top of the stack.
func (a *App) push(s Screen) { a.stack = append(a.stack, s) }

// pop removes the top screen unless it is the only remaining one.
func (a *App) pop() {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
	}
}

// top returns the currently visible screen (top of stack).
func (a App) top() Screen {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

// currentRepo reports whether a repo context has been set.
func (a App) currentRepo() (gh.RepoRef, bool) {
	if a.repo == nil {
		return gh.RepoRef{}, false
	}
	return *a.repo, true
}

// replaceTop swaps the top-of-stack screen in place.
func (a *App) replaceTop(s Screen) {
	if len(a.stack) == 0 {
		a.stack = []Screen{s}
		return
	}
	a.stack[len(a.stack)-1] = s
}

// Update handles global keys and navigation, delegating the rest to the top screen.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		if s := a.top(); s != nil {
			ns, cmd := s.Update(msg)
			a.replaceTop(ns)
			return a, cmd
		}
		return a, nil

	case tea.KeyMsg:
		// If the top screen is capturing text input (e.g. dashboard filter),
		// delegate the key directly without checking global shortcuts.
		if c, ok := a.top().(inputCapturer); ok && c.capturingInput() {
			ns, cmd := a.top().Update(m)
			a.replaceTop(ns)
			return a, cmd
		}

		switch m.String() {
		case keyQuit, "ctrl+c":
			return a, tea.Quit
		case keyBack:
			a.pop()
			return a, nil
		case keyHelp:
			a.showHelp = !a.showHelp
			return a, nil
		case keyRepos:
			return a.handleGoto(gotoReposMsg{})
		case keyWorkflows:
			if _, ok := a.currentRepo(); ok {
				return a.handleGoto(gotoWorkflowsMsg{})
			}
			return a, nil // no repo selected: swallow, don't leak the key to the screen
		case keyRuns:
			if _, ok := a.currentRepo(); ok {
				return a.handleGoto(gotoRunsMsg{})
			}
			return a, nil // no repo selected: swallow, don't leak the key to the screen
		}

	case pushMsg:
		a.push(m.screen)
		return a, nil
	case popMsg:
		a.pop()
		return a, nil
	case setRepoMsg:
		r := m.repo
		a.repo = &r
		return a, nil
	case enterRepoMsg:
		repo := m.repo
		a.repo = &repo
		runs, cmd := newRuns(a.client, repo, a.cfg.RunListLimit)
		a.push(runs)
		return a, cmd
	case gotoReposMsg, gotoWorkflowsMsg, gotoRunsMsg:
		return a.handleGoto(msg)
	case errMsg:
		if m.err != nil {
			a.errText = m.err.Error()
		}
		return a, nil
	case clearErrMsg:
		a.errText = ""
		return a, nil
	}

	// Delegate everything else to the top screen.
	if s := a.top(); s != nil {
		ns, cmd := s.Update(msg)
		a.replaceTop(ns)
		return a, cmd
	}
	return a, nil
}

// handleGoto resolves a global navigation message into stack operations.
func (a App) handleGoto(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case gotoReposMsg:
		dash, cmd := newDashboard(a.client, a.cfg)
		a.repo = nil
		a.stack = []Screen{dash}
		return a, cmd
	case gotoWorkflowsMsg:
		repo, ok := a.currentRepo()
		if !ok {
			return a, nil
		}
		wf, cmd := newWorkflows(a.client, repo)
		a.stack = a.stack[:1]
		a.push(wf)
		return a, cmd
	case gotoRunsMsg:
		repo, ok := a.currentRepo()
		if !ok {
			return a, nil
		}
		runs, cmd := newRuns(a.client, repo, a.cfg.RunListLimit)
		a.stack = a.stack[:1]
		a.push(runs)
		return a, cmd
	}
	return a, nil
}

// breadcrumb builds the header path from repo + stack titles.
func (a App) breadcrumb() string {
	parts := []string{"ghrun"}
	if repo, ok := a.currentRepo(); ok {
		parts = append(parts, repo.String())
	}
	for _, s := range a.stack {
		if t := s.Title(); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " › ")
}

// footer renders the key-hint line with an optional error banner.
func (a App) footer() string {
	keys := "[W]orkflows  [U] runs  [R]epos  ·  esc back  ?  help  q quit"
	if a.errText != "" {
		return errStyle.Render("⚠ "+a.errText) + "\n" + footerStyle.Render(keys)
	}
	return footerStyle.Render(keys)
}

// View renders breadcrumb + active screen + footer.
func (a App) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(a.breadcrumb()))
	b.WriteString("\n\n")
	if s := a.top(); s != nil {
		b.WriteString(s.View())
	}
	b.WriteString("\n\n")
	b.WriteString(a.footer())
	return b.String()
}
