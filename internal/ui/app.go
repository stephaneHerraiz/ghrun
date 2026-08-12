package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// inputCapturer is implemented by screens that consume raw keystrokes (e.g. a
// filter box), so the app shell must not treat those keys as global shortcuts.
type inputCapturer interface{ capturingInput() bool }

// errBannerTTL is how long a non-fatal error stays in the footer before it
// auto-clears.
const errBannerTTL = 6 * time.Second

// App is the root Bubbletea model: it owns the screen stack and global chrome.
type App struct {
	client     GHClient
	cfg        config.Config
	stack      []Screen
	repo       *gh.RepoRef // current repo context (nil at dashboard)
	width      int
	height     int
	errText    string
	showHelp   bool
	saveConfig func(config.Config) error
	explainSvc ExplainService // nil when explain is disabled or unconfigured

	chatCacheDir string // empty: chat unavailable
	suspended    bool   // true while an external process owns the terminal
}

// homeScreen returns the home/initial screen: the org picker until a default
// org is chosen, then the multi-repo dashboard.
func (a App) homeScreen() (Screen, tea.Cmd) {
	if a.cfg.DefaultOrg == "" {
		return newOrgPicker(a.client)
	}
	return newDashboard(a.client, a.cfg)
}

// NewApp builds the root model seeding the initial screen via homeScreen.
func NewApp(c GHClient, cfg config.Config) App {
	a := App{
		client:     c,
		cfg:        cfg,
		saveConfig: func(cf config.Config) error { return cf.Save() },
	}
	s, _ := a.homeScreen()
	a.stack = []Screen{s}
	return a
}

// WithExplainService enables the run-failure explanation feature. Screens
// never hold the service themselves: rundetail emits explainRunMsg and the
// App resolves it here.
func (a App) WithExplainService(s ExplainService) App {
	a.explainSvc = s
	return a
}

// WithChatCacheDir enables the "discuss this run with Claude" feature by
// giving the App somewhere to write per-run context files.
func (a App) WithChatCacheDir(dir string) App {
	a.chatCacheDir = dir
	return a
}

// chatEnabled reports whether the chat feature can be offered.
func (a App) chatEnabled() bool {
	return a.chatCacheDir != "" && a.cfg.Chat.IsEnabled()
}

// tickInterval is the app's base polling cadence, floored for safety.
func (a App) tickInterval() time.Duration {
	s := a.cfg.RefreshIntervalSeconds
	if s < 1 {
		s = 4
	}
	return time.Duration(s) * time.Second
}

// Init kicks off the initial screen's load and starts the single app ticker.
func (a App) Init() tea.Cmd {
	var initCmd tea.Cmd
	switch s := a.top().(type) {
	case *dashboard:
		initCmd = s.initCmd()
	case *orgpicker:
		initCmd = s.initCmd()
	}
	return tea.Batch(initCmd, tickCmd(a.tickInterval()))
}

// push appends a screen to the top of the stack.
func (a *App) push(s Screen) { a.stack = append(a.stack, a.show(s)) }

// show delivers the current terminal size to a screen as it becomes visible, so
// a freshly created screen renders at full width without waiting for the next
// resize event. A zero size (before the first WindowSizeMsg) is left untouched.
func (a *App) show(s Screen) Screen {
	if s == nil || a.width <= 0 {
		return s
	}
	ns, _ := s.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	return ns
}

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
		// Deliver the new size to every stacked screen so a buried screen is
		// correctly sized when it is revealed by a pop, not just the top one.
		for i, s := range a.stack {
			ns, _ := s.Update(msg)
			a.stack[i] = ns
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

	case orgSelectedMsg:
		a.cfg.DefaultOrg = m.org
		var saveCmd tea.Cmd
		if a.saveConfig != nil {
			if err := a.saveConfig(a.cfg); err != nil {
				saveCmd = func() tea.Msg { return errMsg{err: fmt.Errorf("saving config: %w", err)} }
			}
		}
		dash, dashCmd := newDashboard(a.client, a.cfg)
		a.repo = nil
		a.stack = []Screen{a.show(dash)}
		return a, tea.Batch(saveCmd, dashCmd)
	case favoritesChangedMsg:
		a.cfg.Favorites = m.favorites
		if a.saveConfig != nil {
			if err := a.saveConfig(a.cfg); err != nil {
				return a, func() tea.Msg { return errMsg{err: fmt.Errorf("saving favorites: %w", err)} }
			}
		}
		return a, nil
	case pushMsg:
		// Stamp feature availability on screens that surface it contextually.
		if rd, ok := m.screen.(*rundetail); ok {
			rd.explainAvailable = a.explainSvc != nil
			rd.chatAvailable = a.chatEnabled()
		}
		a.push(m.screen)
		return a, nil
	case explainRunMsg:
		if a.explainSvc == nil {
			return a, nil
		}
		es, cmd := newExplain(a.client, a.explainSvc, m.repo, m.id, m.detail)
		a.push(es)
		return a, cmd
	case explainLogLoadedMsg, explainLocalMsg, explainClaudeMsg:
		// Explain results must reach the explain screen even when it is
		// buried (e.g. raw logs were opened while claude was thinking) —
		// data messages otherwise only reach the top screen. Matching on the
		// run id keeps a late result from a popped screen out of a newer one.
		id := explainMsgID(msg)
		for i := len(a.stack) - 1; i >= 0; i-- {
			if es, ok := a.stack[i].(*explainScreen); ok && es.id == id {
				ns, cmd := a.stack[i].Update(msg)
				a.stack[i] = ns
				return a, cmd
			}
		}
		return a, nil
	case enterRepoMsg:
		repo := m.repo
		a.repo = &repo
		runs, cmd := newRuns(a.client, repo, a.cfg.RunListLimit, a.cfg.ListPageSize)
		a.push(runs)
		return a, cmd
	case chatRequestMsg:
		if !a.chatEnabled() {
			return a, nil
		}
		return a, prepareChatCmd(a.client, a.cfg.Chat, a.chatCacheDir, m.repo, m.id, m.detail)
	case chatReadyMsg:
		// tea.ExecProcess hands the terminal to claude. The ticker is parked
		// meanwhile: it would otherwise keep firing gh calls for a screen
		// nobody can see, and the resumed chain would run alongside the one
		// chatDoneMsg starts.
		a.suspended = true
		return a, tea.ExecProcess(m.cmd, func(err error) tea.Msg { return chatDoneMsg{err: err} })
	case chatDoneMsg:
		a.suspended = false
		var clear tea.Cmd
		if m.err != nil {
			// Set the banner here rather than emitting an errMsg: the ticker
			// restart below must go out in the same batch, and the auto-clear
			// mirrors what the errMsg case does.
			a.errText = fmt.Errorf("chat: %w", m.err).Error()
			clear = tea.Tick(errBannerTTL, func(time.Time) tea.Msg { return clearErrMsg{} })
		}
		var reload tea.Cmd
		if r, ok := a.top().(refresher); ok {
			reload, _ = r.refresh()
		}
		return a, tea.Batch(clear, reload, tickCmd(a.tickInterval()))

	case tickMsg:
		if a.suspended {
			return a, nil // chatDoneMsg restarts the chain
		}
		// The app owns the single ticker: only the top screen refreshes, so buried
		// screens' chains can't multiply onto the top screen and over-poll gh.
		next := a.tickInterval()
		var reload tea.Cmd
		if r, ok := a.top().(refresher); ok {
			reload, next = r.refresh()
		}
		if next < time.Second {
			next = time.Second
		}
		return a, tea.Batch(reload, tickCmd(next))
	case errMsg:
		if m.err != nil {
			a.errText = m.err.Error()
			return a, tea.Tick(errBannerTTL, func(time.Time) tea.Msg { return clearErrMsg{} })
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
		s, cmd := a.homeScreen()
		a.repo = nil
		a.stack = []Screen{a.show(s)}
		return a, cmd
	case gotoWorkflowsMsg:
		repo, ok := a.currentRepo()
		if !ok {
			return a, nil
		}
		wf, cmd := newWorkflows(a.client, repo, a.cfg.ListPageSize)
		a.stack = a.stack[:1]
		a.push(wf)
		return a, cmd
	case gotoRunsMsg:
		repo, ok := a.currentRepo()
		if !ok {
			return a, nil
		}
		runs, cmd := newRuns(a.client, repo, a.cfg.RunListLimit, a.cfg.ListPageSize)
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

// footer renders the key-hint line with an optional error banner. When help is
// toggled (?), it expands into a multi-line key reference.
func (a App) footer() string {
	var keys string
	if a.showHelp {
		keys = strings.Join([]string{
			"Navigation: [W] workflows · [U] runs · [R] repos (home) · esc back · q quit",
			"Lists: ↑/↓ move · Enter open · f favorite (home) · / filter (home)",
			"Runs: r rerun · f rerun-failed · x cancel · o open web · l logs · e explain · c chat · g refresh",
			"? hide help",
		}, "\n")
	} else {
		keys = "[W]orkflows  [U] runs  [R]epos  ·  esc back  ?  help  q quit"
	}
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
