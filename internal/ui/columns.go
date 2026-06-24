package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// padCell fits s into exactly w display columns: it truncates with an ellipsis
// when s is too wide and right-pads with spaces when it is too narrow. It is
// ANSI-aware, so styled cells (e.g. a coloured status icon) measure by their
// visible width, not their escape-sequence length. w <= 0 yields "".
func padCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}
