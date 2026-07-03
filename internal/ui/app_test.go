package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func newTestApp() App {
	cfg := config.Default()
	cfg.DefaultOrg = "test-org" // non-empty → dashboard root (not the org picker)
	a := NewApp(nil, cfg)
	a.width, a.height = 80, 24
	return a
}

func TestPushPopStack(t *testing.T) {
	a := newTestApp()
	a.push(stubScreen{"one"})
	a.push(stubScreen{"two"})
	if a.top().Title() != "two" {
		t.Fatalf("top = %q, want two", a.top().Title())
	}
	a.pop()
	if a.top().Title() != "one" {
		t.Fatalf("after pop top = %q, want one", a.top().Title())
	}
	// Popping the last screen is a no-op (root stays).
	a.pop()
	if len(a.stack) != 1 {
		t.Fatalf("stack len = %d, want 1", len(a.stack))
	}
}

func TestQuitKey(t *testing.T) {
	a := newTestApp()
	a.push(stubScreen{"root"})
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("quit command returned nil msg")
	}
	// tea.Quit returns tea.QuitMsg
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}

func TestEscPopsScreen(t *testing.T) {
	a := newTestApp()
	a.push(stubScreen{"root"})
	a.push(stubScreen{"child"})
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := model.(App)
	if got.top().Title() != "root" {
		t.Fatalf("after esc top = %q, want root", got.top().Title())
	}
}

func TestErrMsgSetsFooterError(t *testing.T) {
	a := newTestApp()
	a.push(stubScreen{"root"})
	model, _ := a.Update(errMsg{err: errorString("boom")})
	got := model.(App)
	if !strings.Contains(got.View(), "boom") {
		t.Fatalf("view missing error; view=\n%s", got.View())
	}
}

func TestAppTickReArmsAndRefreshesTop(t *testing.T) {
	a := newTestApp() // dashboard (a refresher) on the stack
	_, cmd := a.Update(tickMsg(time.Time{}))
	if cmd == nil {
		t.Fatal("app tick must re-arm the single ticker")
	}
	// A non-refresher top (stubScreen) must still keep the ticker alive.
	a.push(stubScreen{"x"})
	_, cmd = a.Update(tickMsg(time.Time{}))
	if cmd == nil {
		t.Fatal("app tick must re-arm even when top is not a refresher")
	}
}

// TestFilteringDashboardSwallowsGlobalKeys verifies that while the dashboard is
// capturing filter input, a global key like "q" is delegated to the screen
// (not treated as quit).
func TestFilteringDashboardSwallowsGlobalKeys(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultOrg = "test-org"
	cfg.Favorites = []string{"o/alpha", "o/beta"}
	var m tea.Model = NewApp(nil, cfg)

	// Drive '/' through the app so the routing path is exercised. '/' is not a
	// global key, so it delegates to the dashboard, which enters filtering.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Confirm the dashboard (top of stack) is now capturing input.
	d, ok := m.(App).top().(*dashboard)
	if !ok {
		t.Fatal("top screen is not a *dashboard")
	}
	if !d.capturingInput() {
		t.Fatal("dashboard should be in filtering mode after '/'")
	}

	// Now type 'q' — must NOT quit; the app must delegate it to the dashboard.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("'q' while filtering should not quit")
		}
	}
}

func TestAppPersistsFavoritesChange(t *testing.T) {
	a := newTestApp()
	var saved config.Config
	called := false
	a.saveConfig = func(c config.Config) error { saved = c; called = true; return nil }

	model, _ := a.Update(favoritesChangedMsg{favorites: []string{"acme/tool"}})
	got := model.(App)

	if !called {
		t.Fatal("favoritesChangedMsg should trigger a config save")
	}
	if len(saved.Favorites) != 1 || saved.Favorites[0] != "acme/tool" {
		t.Fatalf("saved favorites = %v, want [acme/tool]", saved.Favorites)
	}
	if len(got.cfg.Favorites) != 1 || got.cfg.Favorites[0] != "acme/tool" {
		t.Fatalf("in-memory favorites = %v, want [acme/tool]", got.cfg.Favorites)
	}
}

func TestAppDeliversCurrentSizeToPushedScreen(t *testing.T) {
	a := newTestApp() // width/height seeded to 80/24
	rec := &sizeRecordingScreen{}
	a.push(rec)
	if rec.gotWidth != 80 || rec.gotHeight != 24 {
		t.Fatalf("pushed screen received %dx%d, want 80x24 (size must be delivered on push)", rec.gotWidth, rec.gotHeight)
	}
}

func TestAppResizePropagatesToAllStackedScreens(t *testing.T) {
	a := newTestApp()
	r1, r2 := &sizeRecordingScreen{}, &sizeRecordingScreen{}
	a.stack = []Screen{r1, r2}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	for i, r := range []*sizeRecordingScreen{r1, r2} {
		if r.gotWidth != 120 || r.gotHeight != 40 {
			t.Fatalf("stacked screen %d got %dx%d, want 120x40", i, r.gotWidth, r.gotHeight)
		}
	}
}

func TestNewAppShowsOrgPickerWhenNoDefaultOrg(t *testing.T) {
	// empty default org → picker
	a := NewApp(nil, config.Config{})
	if _, ok := a.top().(*orgpicker); !ok {
		t.Fatalf("top = %T, want *orgpicker when DefaultOrg empty", a.top())
	}
	// non-empty default org → dashboard
	b := NewApp(nil, config.Config{DefaultOrg: "acme"})
	if _, ok := b.top().(*dashboard); !ok {
		t.Fatalf("top = %T, want *dashboard when DefaultOrg set", b.top())
	}
}

func TestOrgSelectedSavesAndSwapsToDashboard(t *testing.T) {
	a := NewApp(nil, config.Config{}) // org picker root
	var saved config.Config
	a.saveConfig = func(c config.Config) error { saved = c; return nil }

	model, _ := a.Update(orgSelectedMsg{org: "acme"})
	got := model.(App)
	if saved.DefaultOrg != "acme" {
		t.Errorf("saved DefaultOrg = %q, want acme", saved.DefaultOrg)
	}
	if got.cfg.DefaultOrg != "acme" {
		t.Errorf("in-memory DefaultOrg = %q, want acme", got.cfg.DefaultOrg)
	}
	if _, ok := got.top().(*dashboard); !ok {
		t.Fatalf("top after selection = %T, want *dashboard", got.top())
	}
}

func TestPushMarksRunDetailExplainAvailable(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithExplainService(&fakeExplainService{})
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 1)
	m, _ := a.Update(pushMsg{screen: rd})
	a2 := m.(App)
	got, ok := a2.top().(*rundetail)
	if !ok {
		t.Fatalf("top = %T, want *rundetail", a2.top())
	}
	if !got.explainAvailable {
		t.Error("pushed rundetail must have explainAvailable set")
	}

	b := NewApp(nil, config.Config{DefaultOrg: "acme"})
	rd2, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 2)
	m2, _ := b.Update(pushMsg{screen: rd2})
	if m2.(App).top().(*rundetail).explainAvailable {
		t.Error("no service: explainAvailable must stay false")
	}
}

func TestExplainRunMsgPushesExplainScreen(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithExplainService(&fakeExplainService{})
	m, cmd := a.Update(explainRunMsg{repo: gh.RepoRef{Owner: "o", Name: "r"}, id: 5})
	if cmd == nil {
		t.Error("pushing explain must start the log fetch")
	}
	if _, ok := m.(App).top().(*explainScreen); !ok {
		t.Fatalf("top = %T", m.(App).top())
	}
}

func TestExplainRunMsgIgnoredWithoutService(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"})
	before := len(a.stack)
	m, _ := a.Update(explainRunMsg{repo: gh.RepoRef{Owner: "o", Name: "r"}, id: 5})
	if len(m.(App).stack) != before {
		t.Error("explainRunMsg without a service must be a no-op")
	}
}

func TestExplainMsgsReachBuriedExplainScreen(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithExplainService(&fakeExplainService{})
	es, _ := newExplain(nil, &fakeExplainService{}, gh.RepoRef{Owner: "o", Name: "r"}, 5, failedDetail())
	m, _ := a.Update(pushMsg{screen: es})
	es.Update(explainLogLoadedMsg{text: "Error: boom"})
	es.Update(explainLocalMsg{res: explain.LocalResult{}}) // miss -> asking claude
	m, _ = m.(App).Update(pushMsg{screen: stubScreen{title: "logs"}})
	m, _ = m.(App).Update(explainClaudeMsg{res: explain.ClaudeResult{Explanation: "late answer", Source: "claude-sonnet-5"}})
	a2 := m.(App)
	buried, ok := a2.stack[len(a2.stack)-2].(*explainScreen)
	if !ok {
		t.Fatalf("stack[-2] = %T, want *explainScreen", a2.stack[len(a2.stack)-2])
	}
	if !strings.Contains(buried.View(), "late answer") {
		t.Errorf("buried explain screen missed the claude result:\n%s", buried.View())
	}
}
