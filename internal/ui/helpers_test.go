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

// sizeRecordingScreen records the last terminal size delivered to it, so tests
// can assert the app propagates the window size to screens.
type sizeRecordingScreen struct {
	gotWidth, gotHeight int
}

func (s *sizeRecordingScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		s.gotWidth, s.gotHeight = m.Width, m.Height
	}
	return s, nil
}
func (s *sizeRecordingScreen) View() string  { return "sized" }
func (s *sizeRecordingScreen) Title() string { return "sized" }
