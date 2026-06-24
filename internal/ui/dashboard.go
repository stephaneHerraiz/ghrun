package ui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type repoStatus struct {
	repo   gh.RepoRef
	latest *gh.Run
	active int
	err    error
}

type dashboardLoadedMsg struct{ statuses []repoStatus }

type dashboard struct {
	client    GHClient
	cfg       config.Config
	favs      []gh.RepoRef
	statuses  []repoStatus
	cursor    int
	filter    string
	filtering bool
	loading   bool
}

// capturingInput reports whether the dashboard is in text-entry (filter) mode,
// so the app shell can route raw keystrokes here instead of treating them as
// global navigation shortcuts.
func (d *dashboard) capturingInput() bool { return d.filtering }

// newDashboard builds the dashboard from configured favorites.
func newDashboard(c GHClient, cfg config.Config) (*dashboard, tea.Cmd) {
	var favs []gh.RepoRef
	for _, s := range cfg.Favorites {
		if r, err := gh.ParseRepoRef(s); err == nil {
			favs = append(favs, r)
		}
	}
	d := &dashboard{client: c, cfg: cfg, favs: favs, loading: true}
	return d, d.initCmd()
}

func (d *dashboard) initCmd() tea.Cmd {
	return d.loadCmd()
}

func (d *dashboard) interval() time.Duration {
	s := d.cfg.RefreshIntervalSeconds
	if s < 1 {
		s = 4
	}
	return time.Duration(s) * time.Second
}

// refresh reloads the favorites' statuses while any run is active; driven by the
// app's single ticker. Returns nil + a slow interval when nothing is active.
func (d *dashboard) refresh() (tea.Cmd, time.Duration) {
	if d.anyActive() {
		return d.loadCmd(), d.interval()
	}
	return nil, 15 * time.Second
}

// loadCmd fans out ListRuns over all favorites concurrently.
func (d *dashboard) loadCmd() tea.Cmd {
	favs := d.favs
	c := d.client
	return func() tea.Msg {
		statuses := make([]repoStatus, len(favs))
		var wg sync.WaitGroup
		for i, repo := range favs {
			wg.Add(1)
			go func(i int, repo gh.RepoRef) {
				defer wg.Done()
				runs, err := c.ListRuns(repo, 20)
				if err != nil {
					statuses[i] = repoStatus{repo: repo, err: err}
					return
				}
				statuses[i] = aggregateRepoStatus(repo, runs)
			}(i, repo)
		}
		wg.Wait()
		return dashboardLoadedMsg{statuses: statuses}
	}
}

// aggregateRepoStatus reduces a repo's runs to its latest run + active count.
func aggregateRepoStatus(repo gh.RepoRef, runs []gh.Run) repoStatus {
	st := repoStatus{repo: repo}
	for i := range runs {
		if runs[i].Active() {
			st.active++
		}
	}
	if len(runs) > 0 {
		latest := runs[0]
		st.latest = &latest
	}
	return st
}

func (d *dashboard) anyActive() bool {
	for _, s := range d.statuses {
		if s.active > 0 {
			return true
		}
	}
	return false
}

func (d *dashboard) visible() []repoStatus {
	if d.filter == "" {
		return d.statuses
	}
	var out []repoStatus
	for _, s := range d.statuses {
		if strings.Contains(strings.ToLower(s.repo.String()), strings.ToLower(d.filter)) {
			out = append(out, s)
		}
	}
	return out
}

func (d *dashboard) Title() string { return "" } // root: no breadcrumb segment

func (d *dashboard) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case dashboardLoadedMsg:
		d.statuses = m.statuses
		d.loading = false
		// re-sort: active repos first, then by name
		sort.SliceStable(d.statuses, func(i, j int) bool {
			if (d.statuses[i].active > 0) != (d.statuses[j].active > 0) {
				return d.statuses[i].active > 0
			}
			return d.statuses[i].repo.String() < d.statuses[j].repo.String()
		})
		if n := len(d.visible()); d.cursor >= n {
			d.cursor = 0
		}
		return d, nil
	case tea.KeyMsg:
		return d.handleKey(m)
	}
	return d, nil
}

func (d *dashboard) handleKey(m tea.KeyMsg) (Screen, tea.Cmd) {
	vis := d.visible()

	if d.filtering {
		switch m.Type {
		case tea.KeyEsc:
			// Exit filter mode and clear the filter.
			d.filtering = false
			d.filter = ""
			d.cursor = 0
		case tea.KeyEnter:
			// Confirm: exit filter mode, keep the filter, emit enterRepoMsg if a row is selected.
			d.filtering = false
			if d.cursor < len(vis) {
				repo := vis[d.cursor].repo
				return d, func() tea.Msg { return enterRepoMsg{repo: repo} }
			}
		case tea.KeyBackspace:
			if len(d.filter) > 0 {
				runes := []rune(d.filter)
				d.filter = string(runes[:len(runes)-1])
				d.cursor = 0
			}
		case tea.KeyRunes:
			if len(m.Runes) == 1 {
				d.filter += string(m.Runes)
				d.cursor = 0
			}
		case tea.KeyUp:
			if d.cursor > 0 {
				d.cursor--
			}
		case tea.KeyDown:
			vis = d.visible() // recompute after any filter change
			if d.cursor < len(vis)-1 {
				d.cursor++
			}
		}
		return d, nil
	}

	// Normal (non-filtering) mode.
	switch m.String() {
	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "j":
		if d.cursor < len(vis)-1 {
			d.cursor++
		}
	case "g":
		return d, d.loadCmd()
	case "enter":
		if d.cursor < len(vis) {
			repo := vis[d.cursor].repo
			return d, func() tea.Msg { return enterRepoMsg{repo: repo} }
		}
	case "/":
		d.filtering = true
	}
	return d, nil
}

// enterRepoMsg asks the App to set the repo and open its runs screen.
type enterRepoMsg struct{ repo gh.RepoRef }

func (d *dashboard) View() string {
	if d.loading && len(d.statuses) == 0 {
		return "Chargement des favoris…"
	}
	if len(d.favs) == 0 {
		return "Aucun favori configuré.\nÉditez ~/.config/ghrun/config.yaml (clé favorites)."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%-40s %-26s %-8s %s\n", "REPO", "DERNIER RUN", "ÉTAT", "ACTIFS"))
	for i, s := range d.visible() {
		cursor := "  "
		if i == d.cursor {
			cursor = "▸ "
		}
		last, state := "—", "—"
		if s.err != nil {
			last, state = "erreur", "!"
		} else if s.latest != nil {
			last = fmt.Sprintf("%s · %s", s.latest.WorkflowName, s.latest.HeadBranch)
			state = statusIcon(s.latest.Status, s.latest.Conclusion)
		}
		active := "–"
		if s.active > 0 {
			active = fmt.Sprintf("%d", s.active)
		}
		b.WriteString(fmt.Sprintf("%s%-40s %-26s %-8s %s\n", cursor, s.repo.String(), last, state, active))
	}
	if d.filtering {
		b.WriteString("\nfiltre: " + d.filter + "▌")
	} else if d.filter != "" {
		b.WriteString("\nfiltre: " + d.filter)
	}
	b.WriteString("\n[Enter] entrer  ·  [g] refresh  ·  [/] filtrer")
	return b.String()
}
