package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOrgPickerLoadsAndSelects(t *testing.T) {
	p, _ := newOrgPicker(nil)
	s, _ := p.Update(namespacesLoadedMsg{names: []string{"alice", "acme"}})
	op := s.(*orgpicker)
	if len(op.items) != 2 {
		t.Fatalf("items = %v", op.items)
	}
	if op.Title() == "" {
		t.Error("picker should have a non-empty title")
	}
	// down → select "acme" → Enter emits orgSelectedMsg{acme}
	s, _ = op.Update(tea.KeyMsg{Type: tea.KeyDown})
	op = s.(*orgpicker)
	_, cmd := op.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should emit a selection command")
	}
	msg := cmd()
	sel, ok := msg.(orgSelectedMsg)
	if !ok || sel.org != "acme" {
		t.Fatalf("selection = %#v, want orgSelectedMsg{acme}", msg)
	}
}

func TestOrgPickerShowsError(t *testing.T) {
	p, _ := newOrgPicker(nil)
	s, _ := p.Update(namespacesLoadedMsg{err: errorString("network down")})
	op := s.(*orgpicker)
	if !strings.Contains(op.View(), "network down") {
		t.Fatalf("view should show the error; got:\n%s", op.View())
	}
}
