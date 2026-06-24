package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPadCellPadsShortStrings(t *testing.T) {
	got := padCell("ab", 5)
	if got != "ab   " {
		t.Fatalf("padCell(\"ab\", 5) = %q, want %q", got, "ab   ")
	}
	if ansi.StringWidth(got) != 5 {
		t.Errorf("width = %d, want 5", ansi.StringWidth(got))
	}
}

func TestPadCellTruncatesLongStrings(t *testing.T) {
	got := padCell("abcdefgh", 5)
	if ansi.StringWidth(got) != 5 {
		t.Fatalf("truncated width = %d, want 5 (got %q)", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("truncation should add an ellipsis; got %q", got)
	}
}

func TestPadCellMeasuresStyledStringByVisibleWidth(t *testing.T) {
	// A coloured icon is one visible column despite the ANSI escape codes;
	// padCell must pad to the requested width based on the visible width.
	icon := okStyle.Render("✓")
	got := padCell(icon, 4)
	if ansi.StringWidth(got) != 4 {
		t.Fatalf("styled cell width = %d, want 4", ansi.StringWidth(got))
	}
	if !strings.Contains(got, "✓") {
		t.Errorf("padding must preserve the icon; got %q", got)
	}
}

func TestPadCellZeroWidth(t *testing.T) {
	if got := padCell("anything", 0); got != "" {
		t.Errorf("padCell(_, 0) = %q, want empty", got)
	}
}
