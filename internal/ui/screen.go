package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Screen is one view in the navigation stack.
type Screen interface {
	// Update handles a message and returns the (possibly new) screen and a command.
	Update(msg tea.Msg) (Screen, tea.Cmd)
	// View renders the screen body (without the global chrome).
	View() string
	// Title is the breadcrumb segment for this screen.
	Title() string
}

// refresher is implemented by screens that want periodic refresh while they are
// the active (top) screen. The app shell owns a single ticker and calls
// refresh() only on the top screen each tick, which prevents buried screens'
// tick chains from multiplying onto the top screen. refresh returns a reload
// command (nil when nothing needs reloading) and the delay until the next tick.
type refresher interface {
	refresh() (tea.Cmd, time.Duration)
}
