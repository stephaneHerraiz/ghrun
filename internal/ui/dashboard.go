package ui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

type repoStatus struct {
	repo   gh.RepoRef
	latest *gh.Run
	active int
	err    error
	isOrg  bool // org repo (name only, no live status) vs. a favorite (live)
}

type dashboardLoadedMsg struct{ statuses []repoStatus }

type dashboard struct {
	client     GHClient
	cfg        config.Config
	org        string // cfg.DefaultOrg: source for the listed org repos
	favs       []gh.RepoRef
	statuses   []repoStatus
	orgRepos   []gh.RepoRef // org repos excluding favorites (name only)
	cachePath  string
	cursor     int
	offset     int // index of the first displayed row (vertical scroll position)
	filter     string
	filtering  bool
	loading    bool // favorites' live status loading
	orgLoading bool // org repo list loading (cache + gh)
	orgFetched bool // a fresh (non-cache) org repo result has been applied
}

// capturingInput reports whether the dashboard is in text-entry (filter) mode,
// so the app shell can route raw keystrokes here instead of treating them as
// global navigation shortcuts.
func (d *dashboard) capturingInput() bool { return d.filtering }

// newDashboard builds the dashboard from configured favorites plus the chosen
// org's repositories (the hybrid multi-repo selector).
func newDashboard(c GHClient, cfg config.Config) (*dashboard, tea.Cmd) {
	var favs []gh.RepoRef
	for _, s := range cfg.Favorites {
		if r, err := gh.ParseRepoRef(s); err == nil {
			favs = append(favs, r)
		}
	}
	cachePath, _ := config.ResolveCachePath() // "" on failure: cache simply disabled
	d := &dashboard{
		client:     c,
		cfg:        cfg,
		org:        cfg.DefaultOrg,
		favs:       favs,
		cachePath:  cachePath,
		loading:    true,
		orgLoading: cfg.DefaultOrg != "",
	}
	return d, d.initCmd()
}

func (d *dashboard) initCmd() tea.Cmd {
	cmds := []tea.Cmd{d.loadCmd()}
	if d.org != "" {
		cmds = append(cmds, loadCachedOrgReposCmd(d.cachePath), loadOrgReposCmd(d.client, d.org, d.cachePath))
	}
	return tea.Batch(cmds...)
}

// refreshOrgReposCmd re-fetches the org repos from gh (used by manual refresh).
func (d *dashboard) refreshOrgReposCmd() tea.Cmd {
	if d.org == "" {
		return nil
	}
	return loadOrgReposCmd(d.client, d.org, d.cachePath)
}

// dedupOrgRepos drops org repos already pinned as favorites and sorts the rest.
func (d *dashboard) dedupOrgRepos(repos []gh.RepoRef) []gh.RepoRef {
	favSet := make(map[string]bool, len(d.favs))
	for _, f := range d.favs {
		favSet[f.String()] = true
	}
	out := make([]gh.RepoRef, 0, len(repos))
	for _, r := range repos {
		if !favSet[r.String()] {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// pageSize is the max number of repo rows shown at once (configurable).
func (d *dashboard) pageSize() int {
	n := d.cfg.DashboardPageSize
	if n < 1 {
		n = 20
	}
	return n
}

// clampOffset keeps a scroll offset within [0, total-page].
func clampOffset(off, page, total int) int {
	return max(0, min(off, max(0, total-page)))
}

// ensureVisible scrolls the window so the cursor row stays in view.
func (d *dashboard) ensureVisible() {
	page := d.pageSize()
	total := len(d.visible())
	if d.cursor < d.offset {
		d.offset = d.cursor
	} else if d.cursor >= d.offset+page {
		d.offset = d.cursor - page + 1
	}
	d.offset = clampOffset(d.offset, page, total)
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

// allRows is the unified, ordered selectable list: favorites (with live status)
// first, then org repos (name only).
func (d *dashboard) allRows() []repoStatus {
	rows := make([]repoStatus, 0, len(d.statuses)+len(d.orgRepos))
	rows = append(rows, d.statuses...)
	for _, r := range d.orgRepos {
		rows = append(rows, repoStatus{repo: r, isOrg: true})
	}
	return rows
}

func (d *dashboard) visible() []repoStatus {
	all := d.allRows()
	if d.filter == "" {
		return all
	}
	var out []repoStatus
	for _, s := range all {
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
		d.ensureVisible()
		return d, nil
	case orgReposLoadedMsg:
		if !m.fromCache {
			d.orgLoading = false
		}
		if m.err != nil {
			// Keep whatever (cached) list we have; surface the error in the footer.
			err := m.err
			return d, func() tea.Msg { return errMsg{err: fmt.Errorf("listing repos for %s: %w", d.org, err)} }
		}
		// A slow, stale cache read must not clobber a fresh gh result — even an
		// empty one (org legitimately has no non-favorite repos), so freshness is
		// tracked by orgFetched, not by the current list length.
		if m.fromCache && d.orgFetched {
			return d, nil
		}
		if !m.fromCache {
			d.orgFetched = true
		}
		d.orgRepos = d.dedupOrgRepos(m.repos)
		if n := len(d.visible()); d.cursor >= n {
			d.cursor = 0
		}
		d.ensureVisible()
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
		d.ensureVisible()
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
		return d, tea.Batch(d.loadCmd(), d.refreshOrgReposCmd())
	case "enter":
		if d.cursor < len(vis) {
			repo := vis[d.cursor].repo
			return d, func() tea.Msg { return enterRepoMsg{repo: repo} }
		}
	case "/":
		d.filtering = true
	}
	d.ensureVisible()
	return d, nil
}

// enterRepoMsg asks the App to set the repo and open its runs screen.
type enterRepoMsg struct{ repo gh.RepoRef }

func (d *dashboard) View() string {
	rows := d.visible()
	if len(rows) == 0 {
		if d.loading || d.orgLoading {
			return "Chargement des repos…"
		}
		if d.filter != "" {
			return "Aucun repo ne correspond au filtre « " + d.filter + " ».\n[esc] effacer le filtre"
		}
		if d.org != "" {
			return fmt.Sprintf("Aucun repo pour %s.\nAjoute des favoris dans ~/.config/ghrun/config.yaml (clé favorites) ou appuie sur [g] pour rafraîchir.", d.org)
		}
		return "Aucun favori configuré.\nÉditez ~/.config/ghrun/config.yaml (clé favorites)."
	}
	// Window the rows to pageSize and follow the cursor.
	page := d.pageSize()
	total := len(rows)
	win := min(page, total)
	offset := clampOffset(d.offset, page, total)
	window := rows[offset : offset+win]
	bar := scrollbarGlyphs(total, win, offset)

	// Build the row lines and a parallel scrollbar column (one glyph per row;
	// the org separator line gets a track glyph so the bar stays continuous).
	var rowLines, barLines []string
	sepDone := false
	for c, s := range window {
		if s.isOrg && !sepDone {
			rowLines = append(rowLines, footerStyle.Render(fmt.Sprintf("  ── repos de %s ──", d.org)))
			if bar != nil {
				barLines = append(barLines, styleGlyph(scrollTrack))
			}
			sepDone = true
		}
		rowLines = append(rowLines, d.rowLine(s, offset+c == d.cursor))
		if bar != nil {
			barLines = append(barLines, styleGlyph(bar[c]))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-40s %-26s %-8s %s\n", "REPO", "DERNIER RUN", "ÉTAT", "ACTIFS")
	list := strings.Join(rowLines, "\n")
	if bar != nil {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, list, "  ", strings.Join(barLines, "\n")))
	} else {
		b.WriteString(list)
	}
	b.WriteByte('\n')
	if total > win {
		fmt.Fprintln(&b, footerStyle.Render(fmt.Sprintf("repos %d–%d / %d", offset+1, offset+win, total)))
	}
	if d.filtering {
		b.WriteString("filtre: " + d.filter + "▌\n")
	} else if d.filter != "" {
		b.WriteString("filtre: " + d.filter + "\n")
	}
	b.WriteString("[Enter] entrer  ·  [g] refresh  ·  [/] filtrer")
	return b.String()
}

// rowLine renders one repo row (a favorite with live status, or a name-only org
// repo which shows placeholder dashes in the status columns).
func (d *dashboard) rowLine(s repoStatus, selected bool) string {
	cursor := "  "
	if selected {
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
	return fmt.Sprintf("%s%-40s %-26s %-8s %s", cursor, s.repo.String(), last, state, active)
}
