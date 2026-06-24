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
	isOrg  bool // org repo (name only, no live status) vs. a favorite (live)
}

type dashboardLoadedMsg struct{ statuses []repoStatus }

type dashboard struct {
	client      GHClient
	cfg         config.Config
	org         string // cfg.DefaultOrg: source for the listed org repos
	favs        []gh.RepoRef
	statuses    []repoStatus
	orgRepos    []gh.RepoRef // org repos excluding favorites (name only)
	orgReposRaw []gh.RepoRef // full fetched org list; re-deduped when favorites change
	cachePath   string
	width       int // terminal width, for full-width column layout (0 = unknown)
	listScroll      // cursor + vertical scroll window
	filter      string
	filtering   bool
	loading     bool // favorites' live status loading
	orgLoading  bool // org repo list loading (cache + gh)
	orgFetched  bool // a fresh (non-cache) org repo result has been applied
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
		listScroll: listScroll{pageSize: cfg.ListPageSize},
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

// clampOffset keeps a scroll offset within [0, total-page].
func clampOffset(off, page, total int) int {
	return max(0, min(off, max(0, total-page)))
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
// Skipped runs (path-filtered or conditionally skipped dispatches) are noise:
// they are excluded when picking the "last run" so the dashboard surfaces the
// most recent run that actually executed.
func aggregateRepoStatus(repo gh.RepoRef, runs []gh.Run) repoStatus {
	st := repoStatus{repo: repo}
	for i := range runs {
		if runs[i].Active() {
			st.active++
		}
	}
	for i := range runs {
		if runs[i].Conclusion == "skipped" {
			continue
		}
		latest := runs[i]
		st.latest = &latest
		break
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
		d.clampCursor(len(d.visible()))
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
		d.orgReposRaw = m.repos
		d.orgRepos = d.dedupOrgRepos(m.repos)
		d.clampCursor(len(d.visible()))
		return d, nil
	case tea.WindowSizeMsg:
		d.width = m.Width
		return d, nil
	case tea.MouseMsg:
		d.handleWheel(m, len(d.visible()))
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
			d.up()
		case tea.KeyDown:
			d.down(len(d.visible())) // recompute after any filter change
		}
		d.ensureVisible(len(d.visible()))
		return d, nil
	}

	// Normal (non-filtering) mode.
	switch m.String() {
	case "up", "k":
		d.up()
	case "down", "j":
		d.down(len(vis))
	case "g":
		return d, tea.Batch(d.loadCmd(), d.refreshOrgReposCmd())
	case "f":
		if d.cursor < len(vis) {
			return d, d.toggleFavorite(vis[d.cursor].repo)
		}
	case "enter":
		if d.cursor < len(vis) {
			repo := vis[d.cursor].repo
			return d, func() tea.Msg { return enterRepoMsg{repo: repo} }
		}
	case "/":
		d.filtering = true
	}
	d.ensureVisible(len(vis))
	return d, nil
}

// enterRepoMsg asks the App to set the repo and open its runs screen.
type enterRepoMsg struct{ repo gh.RepoRef }

// favoritesChangedMsg asks the App to persist the new favorites list to config.
type favoritesChangedMsg struct{ favorites []string }

// toggleFavorite pins or unpins repo as a favorite, updates the visible list
// immediately (keeping the cursor on the toggled repo), and returns a command
// that reloads favorite statuses and persists the change via the App.
func (d *dashboard) toggleFavorite(repo gh.RepoRef) tea.Cmd {
	key := repo.String()
	var favs []gh.RepoRef
	wasFav := false
	for _, f := range d.favs {
		if f.String() == key {
			wasFav = true
			continue // drop it (un-favorite)
		}
		favs = append(favs, f)
	}
	if wasFav {
		d.statuses = removeStatus(d.statuses, key)
	} else {
		favs = append(favs, repo)
		// Show the new favorite right away with a placeholder so it doesn't
		// vanish between leaving the org list and loadCmd refreshing its status.
		d.statuses = append(d.statuses, repoStatus{repo: repo})
	}
	d.favs = favs
	d.cfg.Favorites = favStrings(favs)
	// A removed favorite that belongs to the org reappears in the org list.
	d.orgRepos = d.dedupOrgRepos(d.orgReposRaw)
	d.keepCursorOn(key)
	return tea.Batch(d.loadCmd(), func() tea.Msg { return favoritesChangedMsg{favorites: d.cfg.Favorites} })
}

// keepCursorOn moves the cursor to the row matching key (if still present),
// keeping the highlight on a repo as it moves between sections.
func (d *dashboard) keepCursorOn(key string) {
	vis := d.visible()
	for i, s := range vis {
		if s.repo.String() == key {
			d.cursor = i
			d.ensureVisible(len(vis))
			return
		}
	}
	d.clampCursor(len(vis))
}

// removeStatus returns ss without the entry for the given "owner/name" key.
func removeStatus(ss []repoStatus, key string) []repoStatus {
	var out []repoStatus
	for _, s := range ss {
		if s.repo.String() != key {
			out = append(out, s)
		}
	}
	return out
}

// favStrings renders favorites as "owner/name" config entries.
func favStrings(favs []gh.RepoRef) []string {
	out := make([]string, len(favs))
	for i, f := range favs {
		out[i] = f.String()
	}
	return out
}

// dashLayout holds the per-column display widths for the dashboard table.
type dashLayout struct{ repo, last, state, active int }

const (
	dashStateW  = 5
	dashActiveW = 6
	dashRepoMin = 16
	dashLastMin = 12
)

// dashLayoutFor sizes the four columns to fill width w: REPO and LAST RUN
// absorb the flexible space (REPO ~45%), while STATE and ACTIVE stay fixed. The
// scrollbar gutter (two spaces + one glyph) is reserved only when scrollbar is
// true, so the table fills the full width when no scrollbar is drawn. A width
// of 0 (not yet known, before the first resize) falls back to comfortable fixed
// widths; a known-but-narrow width is clamped to the column floors so the table
// stays as compact as possible rather than overflowing with the wide defaults.
func dashLayoutFor(w int, scrollbar bool) dashLayout {
	if w <= 0 {
		return dashLayout{repo: 40, last: 26, state: dashStateW, active: dashActiveW}
	}
	gutter := 0
	if scrollbar {
		gutter = 3
	}
	// Reserved: cursor marker (2) + three single-space column gaps + the fixed
	// STATE and ACTIVE columns + the optional scrollbar gutter.
	flex := max(dashRepoMin+dashLastMin, w-(2+3+dashStateW+dashActiveW+gutter))
	repo := max(dashRepoMin, flex*45/100)
	return dashLayout{repo: repo, last: flex - repo, state: dashStateW, active: dashActiveW}
}

func (d *dashboard) View() string {
	rows := d.visible()
	if len(rows) == 0 {
		if d.loading || d.orgLoading {
			return "Loading repos…"
		}
		if d.filter != "" {
			return "No repo matches the filter \"" + d.filter + "\".\n[esc] clear the filter"
		}
		if d.org != "" {
			return fmt.Sprintf("No repos for %s.\nAdd favorites in ~/.config/ghrun/config.yaml (favorites key) or press [g] to refresh.", d.org)
		}
		return "No favorites configured.\nEdit ~/.config/ghrun/config.yaml (favorites key)."
	}
	// Window the rows to pageSize and follow the cursor.
	total := len(rows)
	offset, win := d.windowBounds(total)
	window := rows[offset : offset+win]
	bar := scrollbarGlyphs(total, win, offset)
	L := dashLayoutFor(d.width, bar != nil)

	// Build the row lines and a parallel scrollbar column (one glyph per row;
	// the org separator line gets a track glyph so the bar stays continuous).
	var rowLines, barRaw []string
	sepDone := false
	for c, s := range window {
		if s.isOrg && !sepDone {
			rowLines = append(rowLines, d.orgSeparator(L))
			if bar != nil {
				barRaw = append(barRaw, scrollTrack)
			}
			sepDone = true
		}
		rowLines = append(rowLines, d.rowLine(s, offset+c == d.cursor, L))
		if bar != nil {
			barRaw = append(barRaw, bar[c])
		}
	}

	var b strings.Builder
	b.WriteString(d.headerLine(L))
	b.WriteByte('\n')
	b.WriteString(joinScrollbar(rowLines, barRaw))
	b.WriteByte('\n')
	if total > win {
		fmt.Fprintln(&b, footerStyle.Render(fmt.Sprintf("repos %d–%d / %d", offset+1, offset+win, total)))
	}
	if d.filtering {
		b.WriteString("filter: " + d.filter + "▌\n")
	} else if d.filter != "" {
		b.WriteString("filter: " + d.filter + "\n")
	}
	b.WriteString("[Enter] enter  ·  [f] favorite  ·  [g] refresh  ·  [/] filter")
	return b.String()
}

// headerLine renders the column header, indented by the cursor-marker width so
// it aligns with the rows below.
func (d *dashboard) headerLine(L dashLayout) string {
	return fmt.Sprintf("  %s %s %s %s",
		padCell("REPO", L.repo), padCell("LAST RUN", L.last),
		padCell("STATE", L.state), padCell("ACTIVE", L.active))
}

// orgSeparator renders the "repos in <org>" divider, extending the rule to fill
// the table width.
func (d *dashboard) orgSeparator(L dashLayout) string {
	label := fmt.Sprintf("── repos in %s ", d.org)
	rowW := 2 + L.repo + 1 + L.last + 1 + L.state + 1 + L.active // row content, no scrollbar
	label += strings.Repeat("─", max(0, rowW-2-len([]rune(label))))
	return footerStyle.Render("  " + label)
}

// rowLine renders one repo row (a favorite with live status, or a name-only org
// repo which shows placeholder dashes in the status columns), with each column
// padded/truncated to the computed layout so the table fills the width.
func (d *dashboard) rowLine(s repoStatus, selected bool, L dashLayout) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}
	last, state := "—", "—"
	if s.err != nil {
		last, state = "error", "!"
	} else if s.latest != nil {
		last = fmt.Sprintf("%s · %s", s.latest.WorkflowName, s.latest.HeadBranch)
		state = statusIcon(s.latest.Status, s.latest.Conclusion)
	}
	active := "–"
	if s.active > 0 {
		active = fmt.Sprintf("%d", s.active)
	}
	return fmt.Sprintf("%s%s %s %s %s",
		cursor, padCell(s.repo.String(), L.repo), padCell(last, L.last),
		padCell(state, L.state), padCell(active, L.active))
}
