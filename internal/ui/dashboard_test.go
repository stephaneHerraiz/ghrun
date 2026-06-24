package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if !strings.Contains(v, "acme") || !strings.Contains(v, "repos de") {
		t.Fatalf("view should show the org separator; got:\n%s", v)
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
