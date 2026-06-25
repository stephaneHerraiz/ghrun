package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestAggregateRepoStatus(t *testing.T) {
	repo := gh.RepoRef{Owner: "o", Name: "r"}
	runs := []gh.Run{
		{ID: 3, Status: "in_progress", WorkflowName: "CI"},
		{ID: 2, Status: "completed", Conclusion: "success", WorkflowName: "Deploy"},
		{ID: 1, Status: "queued", WorkflowName: "Nightly"},
	}
	st := aggregateRepoStatus(repo, runs)
	if st.latest == nil || st.latest.ID != 3 {
		t.Fatalf("latest = %+v, want id 3", st.latest)
	}
	if st.active != 2 {
		t.Errorf("active = %d, want 2", st.active)
	}
}

func TestAggregateRepoStatusEmpty(t *testing.T) {
	st := aggregateRepoStatus(gh.RepoRef{Owner: "o", Name: "r"}, nil)
	if st.latest != nil || st.active != 0 {
		t.Errorf("empty repo status = %+v", st)
	}
}

func TestAggregateRepoStatusIgnoresSkippedForLatest(t *testing.T) {
	repo := gh.RepoRef{Owner: "o", Name: "r"}
	// Most recent runs were skipped (path-filtered / conditionally skipped);
	// the "last run" should be the most recent run that actually ran.
	runs := []gh.Run{
		{ID: 5, Status: "completed", Conclusion: "skipped", WorkflowName: "CI"},
		{ID: 4, Status: "completed", Conclusion: "skipped", WorkflowName: "CI"},
		{ID: 3, Status: "completed", Conclusion: "success", WorkflowName: "Deploy"},
	}
	st := aggregateRepoStatus(repo, runs)
	if st.latest == nil || st.latest.ID != 3 {
		t.Fatalf("latest = %+v, want the most recent non-skipped run (id 3)", st.latest)
	}
}

func TestAggregateRepoStatusAllSkippedHasNoLatest(t *testing.T) {
	repo := gh.RepoRef{Owner: "o", Name: "r"}
	runs := []gh.Run{
		{ID: 2, Status: "completed", Conclusion: "skipped"},
		{ID: 1, Status: "completed", Conclusion: "skipped"},
	}
	st := aggregateRepoStatus(repo, runs)
	if st.latest != nil {
		t.Fatalf("latest = %+v, want nil when every run was skipped", st.latest)
	}
}

func TestDashboardFilter(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{Favorites: []string{}})

	// Load three repos via dashboardLoadedMsg.
	loadMsg := dashboardLoadedMsg{statuses: []repoStatus{
		{repo: gh.RepoRef{Owner: "o", Name: "alpha"}},
		{repo: gh.RepoRef{Owner: "o", Name: "beta"}},
		{repo: gh.RepoRef{Owner: "o", Name: "gamma"}},
	}}
	sc, _ := d.Update(loadMsg)
	d = sc.(*dashboard)

	// Press '/' to enter filter mode.
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	d = sc.(*dashboard)
	if !d.capturingInput() {
		t.Fatal("expected capturingInput() == true after '/'")
	}

	// Type 'b' then 'e' to filter to "be".
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	d = sc.(*dashboard)
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	d = sc.(*dashboard)

	vis := d.visible()
	if len(vis) != 1 {
		t.Fatalf("visible() len = %d, want 1 (filter=%q)", len(vis), d.filter)
	}
	if vis[0].repo.Name != "beta" {
		t.Errorf("visible()[0].repo.Name = %q, want %q", vis[0].repo.Name, "beta")
	}

	// Press Esc to exit filter mode and clear filter.
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	d = sc.(*dashboard)

	if d.capturingInput() {
		t.Error("capturingInput() should be false after Esc")
	}
	if d.filter != "" {
		t.Errorf("filter should be empty after Esc, got %q", d.filter)
	}
	if len(d.visible()) != 3 {
		t.Errorf("visible() len = %d, want 3 after clearing filter", len(d.visible()))
	}
}

func TestDashboardScrollExitsFilterModeKeepingFilter(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{Favorites: []string{"o/api", "o/web", "o/cli"}})
	sc, _ := d.Update(dashboardLoadedMsg{statuses: []repoStatus{
		{repo: gh.RepoRef{Owner: "o", Name: "api"}},
		{repo: gh.RepoRef{Owner: "o", Name: "web"}},
		{repo: gh.RepoRef{Owner: "o", Name: "cli"}},
	}})
	d = sc.(*dashboard)

	// Enter filter mode and narrow to repos containing "i": api, cli.
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	d = sc.(*dashboard)
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	d = sc.(*dashboard)
	if !d.capturingInput() {
		t.Fatal("expected filter mode after typing")
	}
	if len(d.visible()) != 2 {
		t.Fatalf("visible() = %d, want 2 (api, cli)", len(d.visible()))
	}

	// Scrolling down must leave text-entry (so contextual keys work again) while
	// keeping the filter applied, and move the cursor within the filtered list.
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	d = sc.(*dashboard)
	if d.capturingInput() {
		t.Error("scrolling should exit filter mode (capturingInput == false)")
	}
	if d.filter != "i" {
		t.Errorf("filter should be kept after scroll, got %q", d.filter)
	}
	if len(d.visible()) != 2 || d.cursor != 1 {
		t.Fatalf("filtered list should persist and cursor move: visible=%d cursor=%d, want 2/1", len(d.visible()), d.cursor)
	}

	// 'f' now favorites the highlighted repo (cli) instead of being swallowed.
	cli := d.visible()[d.cursor].repo
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if cmd == nil {
		t.Fatal("f should trigger a favorite-toggle command after exiting filter mode")
	}
	for _, f := range d.favs {
		if f.String() == cli.String() {
			t.Errorf("%s should have been un-favorited by f", cli.String())
		}
	}
}

func TestDashboardWheelExitsFilterModeKeepingFilter(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{Favorites: []string{"o/api", "o/web"}})
	sc, _ := d.Update(dashboardLoadedMsg{statuses: []repoStatus{
		{repo: gh.RepoRef{Owner: "o", Name: "api"}},
		{repo: gh.RepoRef{Owner: "o", Name: "web"}},
	}})
	d = sc.(*dashboard)

	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	d = sc.(*dashboard)
	sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	d = sc.(*dashboard)
	if !d.capturingInput() {
		t.Fatal("expected filter mode after typing")
	}

	// A mouse wheel scroll exits filter mode but keeps the filter applied.
	sc, _ = d.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	d = sc.(*dashboard)
	if d.capturingInput() {
		t.Error("wheel scroll should exit filter mode")
	}
	if d.filter != "a" {
		t.Errorf("filter should be kept after wheel scroll, got %q", d.filter)
	}
}

func TestDashboardIncludesOrgReposAsSelectableRows(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme", Favorites: []string{"acme/fav"}})

	// Favorite gets live status via dashboardLoadedMsg.
	sc, _ := d.Update(dashboardLoadedMsg{statuses: []repoStatus{
		{repo: gh.RepoRef{Owner: "acme", Name: "fav"}},
	}})
	d = sc.(*dashboard)

	// Org repos arrive (from gh, not cache).
	sc, _ = d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{
		{Owner: "acme", Name: "zeta"},
		{Owner: "acme", Name: "beta"},
	}})
	d = sc.(*dashboard)

	vis := d.visible()
	if len(vis) != 3 {
		t.Fatalf("visible() len = %d, want 3 (1 fav + 2 org)", len(vis))
	}
	// Favorite first (not org), org repos after, sorted by name.
	if vis[0].isOrg || vis[0].repo.Name != "fav" {
		t.Errorf("row 0 = %+v, want favorite acme/fav", vis[0])
	}
	if !vis[1].isOrg || vis[1].repo.Name != "beta" {
		t.Errorf("row 1 = %+v, want org acme/beta", vis[1])
	}
	if !vis[2].isOrg || vis[2].repo.Name != "zeta" {
		t.Errorf("row 2 = %+v, want org acme/zeta", vis[2])
	}
}

func TestDashboardDedupsOrgReposAgainstFavorites(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme", Favorites: []string{"acme/alpha"}})

	sc, _ := d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{
		{Owner: "acme", Name: "alpha"}, // already a favorite → dropped
		{Owner: "acme", Name: "beta"},
	}})
	d = sc.(*dashboard)

	if len(d.orgRepos) != 1 || d.orgRepos[0].Name != "beta" {
		t.Fatalf("orgRepos = %+v, want only acme/beta", d.orgRepos)
	}
}

func TestDashboardStaleCacheDoesNotClobberFreshOrgRepos(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme"})

	// Fresh gh result arrives first.
	sc, _ := d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{{Owner: "acme", Name: "fresh"}}})
	d = sc.(*dashboard)

	// Then a (slower) stale cache message arrives — must NOT overwrite fresh data.
	sc, _ = d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{{Owner: "acme", Name: "stale"}}, fromCache: true})
	d = sc.(*dashboard)

	if len(d.orgRepos) != 1 || d.orgRepos[0].Name != "fresh" {
		t.Fatalf("orgRepos = %+v, want fresh data retained over stale cache", d.orgRepos)
	}
}

func TestDashboardEmptyFreshResultNotClobberedByStaleCache(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme"})

	// Fresh gh result wins the race and is legitimately empty (no org repos).
	sc, _ := d.Update(orgReposLoadedMsg{repos: nil})
	d = sc.(*dashboard)

	// A slower stale cache read arrives with leftover repos — must be ignored.
	sc, _ = d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{{Owner: "acme", Name: "deleted"}}, fromCache: true})
	d = sc.(*dashboard)

	if len(d.orgRepos) != 0 {
		t.Fatalf("orgRepos = %+v, want empty (fresh empty result must win over stale cache)", d.orgRepos)
	}
}

func TestDashboardEnterOrgRepoEmitsEnterRepoMsg(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme"})
	sc, _ := d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{{Owner: "acme", Name: "tool"}}})
	d = sc.(*dashboard)

	// Cursor on the single org row, press Enter.
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if cmd == nil {
		t.Fatal("Enter on an org repo should emit a command")
	}
	msg, ok := cmd().(enterRepoMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want enterRepoMsg", cmd())
	}
	if msg.repo.Name != "tool" {
		t.Errorf("entered repo = %s, want acme/tool", msg.repo.String())
	}
}

func TestDashboardViewShowsOrgSeparator(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme"})
	sc, _ := d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{{Owner: "acme", Name: "tool"}}})
	d = sc.(*dashboard)
	v := d.View()
	if !strings.Contains(v, "acme") || !strings.Contains(v, "repos in") {
		t.Fatalf("view should show the org separator; got:\n%s", v)
	}
}

// batchEmitsFavorites reports whether cmd (possibly a tea.Batch) yields a
// favoritesChangedMsg carrying exactly want. Safe to call only when any sibling
// commands in the batch are client-free (e.g. loadCmd with no favorites).
func batchEmitsFavorites(cmd tea.Cmd, want []string) bool {
	if cmd == nil {
		return false
	}
	check := func(m tea.Msg) bool {
		fc, ok := m.(favoritesChangedMsg)
		return ok && slices.Equal(fc.favorites, want)
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil && check(c()) {
				return true
			}
		}
		return false
	}
	return check(msg)
}

func orgRepoNames(rs []gh.RepoRef) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

func TestDashboardToggleFavoriteAddsOrgRepo(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme"})
	sc, _ := d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{{Owner: "acme", Name: "tool"}}})
	d = sc.(*dashboard)

	// Cursor on the single org row; press 'f' to pin it as a favorite.
	sc, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	d = sc.(*dashboard)

	if cmd == nil {
		t.Fatal("toggling a favorite should return a command (reload + persist)")
	}
	if len(d.favs) != 1 || d.favs[0].Name != "tool" {
		t.Fatalf("favs = %+v, want [acme/tool]", d.favs)
	}
	if !slices.Contains(d.cfg.Favorites, "acme/tool") || len(d.cfg.Favorites) != 1 {
		t.Fatalf("cfg.Favorites = %v, want [acme/tool]", d.cfg.Favorites)
	}
	if len(d.orgRepos) != 0 {
		t.Fatalf("orgRepos = %+v, want empty (tool moved to favorites)", d.orgRepos)
	}
	// Still selectable, now as a favorite row (not org), and the cursor stays on it.
	vis := d.visible()
	if len(vis) != 1 || vis[0].isOrg || vis[0].repo.Name != "tool" {
		t.Fatalf("visible = %+v, want a single favorite row for tool", vis)
	}
	if vis[d.cursor].repo.Name != "tool" {
		t.Errorf("cursor should stay on tool, points at %s", vis[d.cursor].repo.String())
	}
}

func TestDashboardToggleFavoriteRemovesAndPersists(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme", Favorites: []string{"acme/tool"}})
	sc, _ := d.Update(dashboardLoadedMsg{statuses: []repoStatus{{repo: gh.RepoRef{Owner: "acme", Name: "tool"}}}})
	d = sc.(*dashboard)
	// The org list also contains the favorite, so un-pinning brings it back.
	sc, _ = d.Update(orgReposLoadedMsg{repos: []gh.RepoRef{{Owner: "acme", Name: "tool"}, {Owner: "acme", Name: "other"}}})
	d = sc.(*dashboard)

	d.cursor = 0 // the favorite row
	sc, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	d = sc.(*dashboard)

	if len(d.favs) != 0 {
		t.Fatalf("favs = %+v, want empty after un-favorite", d.favs)
	}
	if len(d.cfg.Favorites) != 0 {
		t.Fatalf("cfg.Favorites = %v, want empty", d.cfg.Favorites)
	}
	if !slices.Contains(orgRepoNames(d.orgRepos), "tool") {
		t.Fatalf("orgRepos = %v, want tool restored to the org list", orgRepoNames(d.orgRepos))
	}
	// favs is now empty, so loadCmd in the batch is client-free and safe to run.
	if !batchEmitsFavorites(cmd, []string{}) {
		t.Fatal("un-favorite should emit favoritesChangedMsg with the updated (empty) favorites")
	}
}

func TestDashLayoutForFallsBackWhenWidthUnknown(t *testing.T) {
	L := dashLayoutFor(0, false)
	if L.repo != 40 || L.last != 26 {
		t.Fatalf("fallback layout = %+v, want repo 40 / last 26", L)
	}
}

func TestDashLayoutForFillsWidthExactly(t *testing.T) {
	const w = 120
	for _, scrollbar := range []bool{false, true} {
		L := dashLayoutFor(w, scrollbar)
		gutter := 0
		if scrollbar {
			gutter = 3
		}
		// cursor(2) + repo + gap + last + gap + state + gap + active + gutter
		total := 2 + L.repo + 1 + L.last + 1 + L.state + 1 + L.active + gutter
		if total != w {
			t.Fatalf("scrollbar=%v: columns sum to %d, want exactly %d (layout=%+v)", scrollbar, total, w, L)
		}
	}
}

func TestDashLayoutForNarrowRespectsFloorsNotWideDefaults(t *testing.T) {
	L := dashLayoutFor(30, true)
	if L.repo < dashRepoMin || L.last < dashLastMin {
		t.Fatalf("narrow layout must respect column floors; got %+v", L)
	}
	// A known-but-narrow width must NOT fall back to the wide w=0 defaults,
	// which would overflow the terminal.
	if L.repo >= 40 || L.last >= 26 {
		t.Fatalf("narrow layout = %+v, must clamp to floors, not the wide defaults", L)
	}
}

func TestDashboardViewFillsTerminalWidth(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme", Favorites: []string{"acme/fav"}})
	sc, _ := d.Update(dashboardLoadedMsg{statuses: []repoStatus{{repo: gh.RepoRef{Owner: "acme", Name: "fav"}}}})
	d = sc.(*dashboard)
	// Deliver a wide terminal size, as the app does when the screen becomes visible.
	sc, _ = d.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	d = sc.(*dashboard)

	v := d.View()
	// A single favorite does not overflow the page, so no scrollbar is drawn and
	// the row must fill the terminal width exactly (140).
	for line := range strings.SplitSeq(v, "\n") {
		if strings.Contains(line, "acme/fav") {
			if w := lipgloss.Width(line); w != 140 {
				t.Fatalf("repo row width = %d, want exactly 140 (full terminal width)\nline=%q", w, line)
			}
			return
		}
	}
	t.Fatalf("did not find the favorite row in:\n%s", v)
}

func countThumb(g []string) int {
	n := 0
	for _, c := range g {
		if c == scrollThumb {
			n++
		}
	}
	return n
}

func TestScrollbarGlyphs(t *testing.T) {
	// Everything fits → no scrollbar.
	if g := scrollbarGlyphs(3, 3, 0); g != nil {
		t.Errorf("total<=win should give nil, got %v", g)
	}

	// 10 rows, window of 5: thumb height = 25/10 = 2, at top when offset 0.
	top := scrollbarGlyphs(10, 5, 0)
	if len(top) != 5 {
		t.Fatalf("len = %d, want 5", len(top))
	}
	if countThumb(top) != 2 {
		t.Errorf("thumb height = %d, want 2 (glyphs=%v)", countThumb(top), top)
	}
	if top[0] != scrollThumb || top[4] != scrollTrack {
		t.Errorf("offset 0 should put thumb at top; got %v", top)
	}

	// Scrolled to the bottom: thumb sits at the end.
	bot := scrollbarGlyphs(10, 5, 5)
	if bot[len(bot)-1] != scrollThumb || bot[0] != scrollTrack {
		t.Errorf("max offset should put thumb at bottom; got %v", bot)
	}
}

func TestDashboardWindowsAndScrollsWithCursor(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme", ListPageSize: 3})
	repos := []gh.RepoRef{
		{Owner: "acme", Name: "r0"}, {Owner: "acme", Name: "r1"}, {Owner: "acme", Name: "r2"},
		{Owner: "acme", Name: "r3"}, {Owner: "acme", Name: "r4"}, {Owner: "acme", Name: "r5"},
	}
	sc, _ := d.Update(orgReposLoadedMsg{repos: repos})
	d = sc.(*dashboard)

	if d.offset != 0 || d.cursor != 0 {
		t.Fatalf("initial cursor/offset = %d/%d, want 0/0", d.cursor, d.offset)
	}
	// Only pageSize rows are rendered, and the scrollbar thumb is present.
	v := d.View()
	if strings.Count(v, "acme/r") != 3 {
		t.Fatalf("view should show 3 repo rows, found %d:\n%s", strings.Count(v, "acme/r"), v)
	}
	if !strings.Contains(v, scrollThumb) {
		t.Errorf("overflowing list should render a scrollbar thumb; got:\n%s", v)
	}
	if !strings.Contains(v, "/ 6") {
		t.Errorf("view should show the position indicator '… / 6'; got:\n%s", v)
	}

	// Move the cursor to the bottom: the window must scroll.
	for range 5 {
		sc, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
		d = sc.(*dashboard)
	}
	if d.cursor != 5 {
		t.Fatalf("cursor = %d, want 5", d.cursor)
	}
	if d.offset != 3 {
		t.Fatalf("offset = %d, want 3 (window follows cursor to the bottom)", d.offset)
	}
	v = d.View()
	if !strings.Contains(v, "acme/r5") {
		t.Errorf("scrolled view should show the last repo r5; got:\n%s", v)
	}
	if strings.Contains(v, "acme/r0") {
		t.Errorf("scrolled view should NOT show the first repo r0; got:\n%s", v)
	}
}

func TestDashboardMouseWheelScrollsList(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{DefaultOrg: "acme", ListPageSize: 3})
	repos := []gh.RepoRef{
		{Owner: "acme", Name: "r0"}, {Owner: "acme", Name: "r1"}, {Owner: "acme", Name: "r2"},
		{Owner: "acme", Name: "r3"}, {Owner: "acme", Name: "r4"}, {Owner: "acme", Name: "r5"},
	}
	sc, _ := d.Update(orgReposLoadedMsg{repos: repos})
	d = sc.(*dashboard)

	// Wheel down 4 notches → cursor follows and the window scrolls.
	for range 4 {
		sc, _ = d.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
		d = sc.(*dashboard)
	}
	if d.cursor != 4 {
		t.Fatalf("cursor = %d after 4 wheel-down, want 4", d.cursor)
	}
	if d.offset != 2 {
		t.Fatalf("offset = %d, want 2 (window follows cursor)", d.offset)
	}

	// Wheel up 4 notches → back to the top.
	for range 4 {
		sc, _ = d.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
		d = sc.(*dashboard)
	}
	if d.cursor != 0 || d.offset != 0 {
		t.Fatalf("cursor/offset = %d/%d after wheel-up, want 0/0", d.cursor, d.offset)
	}
}

func TestDashboardCursorClampedAfterLoad(t *testing.T) {
	d, _ := newDashboard(nil, config.Config{Favorites: []string{}})
	d.cursor = 2

	// Send a dashboardLoadedMsg with only 1 entry.
	loadMsg := dashboardLoadedMsg{statuses: []repoStatus{
		{repo: gh.RepoRef{Owner: "o", Name: "only"}},
	}}
	sc, _ := d.Update(loadMsg)
	d = sc.(*dashboard)

	if d.cursor != 0 {
		t.Errorf("cursor = %d after clamp, want 0", d.cursor)
	}
}
