package ui

import (
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
