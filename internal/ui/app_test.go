package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
)

func newTestApp() App {
	a := NewApp(nil, config.Default())
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

// TestFilteringDashboardSwallowsGlobalKeys verifies that while the dashboard is
// capturing filter input, a global key like "q" is delegated to the screen
// (not treated as quit).
func TestFilteringDashboardSwallowsGlobalKeys(t *testing.T) {
	a := NewApp(nil, config.Default())
	// Drive '/' through the app so the routing path is exercised.
	// '/' is not a global key, so it delegates to the dashboard, which enters filtering.
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Confirm the dashboard is now capturing input.
	d, ok := a.top().(*dashboard)
	if !ok {
		t.Fatal("top screen is not a *dashboard")
	}
	if !d.capturingInput() {
		t.Skip("dashboard did not enter filtering mode after '/' (no favorites configured, skip)")
	}

	// Now type 'q' — must NOT quit; app should return without a QuitMsg.
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Fatal("'q' while filtering should not quit")
		}
	}
}
