package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listScroll manages a selection cursor and a vertical scroll window over a
// fixed-size list. Screens embed it to get pageSize-limited rendering, a
// proportional scrollbar, and mouse-wheel scrolling with shared behavior.
type listScroll struct {
	cursor   int
	offset   int // index of the first displayed row (vertical scroll position)
	pageSize int
}

// page is the max number of rows shown at once (configurable, floored at 20).
func (l *listScroll) page() int {
	if l.pageSize < 1 {
		return 20
	}
	return l.pageSize
}

func (l *listScroll) up() {
	if l.cursor > 0 {
		l.cursor--
	}
}

func (l *listScroll) down(total int) {
	if l.cursor < total-1 {
		l.cursor++
	}
}

// clampCursor keeps the cursor within the list after a reload, then scrolls the
// window so it stays visible.
func (l *listScroll) clampCursor(total int) {
	if l.cursor >= total {
		l.cursor = 0
	}
	l.ensureVisible(total)
}

// ensureVisible scrolls the window so the cursor row stays in view.
func (l *listScroll) ensureVisible(total int) {
	page := l.page()
	if l.cursor < l.offset {
		l.offset = l.cursor
	} else if l.cursor >= l.offset+page {
		l.offset = l.cursor - page + 1
	}
	l.offset = clampOffset(l.offset, page, total)
}

// handleWheel scrolls on a mouse-wheel event (no-op for other buttons).
func (l *listScroll) handleWheel(m tea.MouseMsg, total int) {
	switch m.Button {
	case tea.MouseButtonWheelUp:
		l.up()
		l.ensureVisible(total)
	case tea.MouseButtonWheelDown:
		l.down(total)
		l.ensureVisible(total)
	}
}

// windowBounds returns the visible slice bounds [offset, offset+win) for total
// rows at the current scroll position.
func (l *listScroll) windowBounds(total int) (offset, win int) {
	page := l.page()
	win = min(page, total)
	offset = clampOffset(l.offset, page, total)
	return offset, win
}

// render windows `lines` (each a fully formatted row including its cursor
// marker) to pageSize, appends a vertical scrollbar when the list overflows and
// a "X–Y / N" position indicator. No trailing newline.
func (l *listScroll) render(lines []string) string {
	total := len(lines)
	offset, win := l.windowBounds(total)
	window := lines[offset : offset+win]
	body := joinScrollbar(window, scrollbarGlyphs(total, win, offset))
	if total > win {
		body += "\n" + footerStyle.Render(fmt.Sprintf("lignes %d–%d / %d", offset+1, offset+win, total))
	}
	return body
}

// joinScrollbar attaches a vertical scrollbar column (one raw glyph per line) to
// the right of a block of lines. bar must be nil (no scrollbar) or the same
// length as lines.
func joinScrollbar(lines, bar []string) string {
	if bar == nil {
		return strings.Join(lines, "\n")
	}
	styled := make([]string, len(bar))
	for i, g := range bar {
		styled[i] = styleGlyph(g)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		strings.Join(lines, "\n"), "  ", strings.Join(styled, "\n"))
}
