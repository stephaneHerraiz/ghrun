package ui

import tea "github.com/charmbracelet/bubbletea"

// Screen is one view in the navigation stack.
type Screen interface {
	// Update handles a message and returns the (possibly new) screen and a command.
	Update(msg tea.Msg) (Screen, tea.Cmd)
	// View renders the screen body (without the global chrome).
	View() string
	// Title is the breadcrumb segment for this screen.
	Title() string
}
