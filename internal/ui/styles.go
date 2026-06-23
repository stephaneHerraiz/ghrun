package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	footerStyle = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	runStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	failStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// statusIcon renders an icon for a (status, conclusion) pair.
func statusIcon(status, conclusion string) string {
	switch status {
	case "completed":
		switch conclusion {
		case "success":
			return okStyle.Render("✓")
		case "failure", "timed_out":
			return failStyle.Render("✗")
		case "cancelled":
			return footerStyle.Render("⊘")
		default:
			return footerStyle.Render("•")
		}
	case "in_progress":
		return runStyle.Render("●")
	case "queued", "waiting", "pending", "":
		return footerStyle.Render("⋯")
	default:
		return footerStyle.Render("•")
	}
}
