package ui

import tea "github.com/charmbracelet/bubbletea"

// stubScreen is a minimal Screen for testing the stack.
type stubScreen struct{ title string }

func (s stubScreen) Update(tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (s stubScreen) View() string                     { return "body:" + s.title }
func (s stubScreen) Title() string                    { return s.title }

// errorString is a tiny error helper for tests.
type errorString string

func (e errorString) Error() string { return string(e) }
