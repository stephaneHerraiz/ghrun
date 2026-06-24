package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	footerStyle = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	runStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	failStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	scrollThumbStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	scrollTrackStyle = lipgloss.NewStyle().Faint(true)
)

// Vertical scrollbar glyphs.
const (
	scrollThumb = "█"
	scrollTrack = "│"
)

// scrollbarGlyphs returns one glyph per visible row (length win) for a list of
// total rows scrolled to offset, or nil when everything fits (total <= win).
// The thumb height is proportional to the visible fraction.
func scrollbarGlyphs(total, win, offset int) []string {
	if win <= 0 || total <= win {
		return nil
	}
	thumb := max(1, win*win/total)
	thumbTop := 0
	if denom := total - win; denom > 0 {
		thumbTop = offset * (win - thumb) / denom
	}
	if thumbTop > win-thumb {
		thumbTop = win - thumb
	}
	if thumbTop < 0 {
		thumbTop = 0
	}
	g := make([]string, win)
	for i := range g {
		if i >= thumbTop && i < thumbTop+thumb {
			g[i] = scrollThumb
		} else {
			g[i] = scrollTrack
		}
	}
	return g
}

// styleGlyph colours a scrollbar glyph (thumb vs track).
func styleGlyph(g string) string {
	if g == scrollThumb {
		return scrollThumbStyle.Render(g)
	}
	return scrollTrackStyle.Render(g)
}

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
