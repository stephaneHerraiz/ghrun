package ui

import (
	"strings"
	"testing"

	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestLogsLoadedSetsContent(t *testing.T) {
	lg, _ := newLogs(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5, true)
	s, _ := lg.Update(logsLoadedMsg{text: "line1\nline2\nERROR boom"})
	l := s.(*logs)
	if !strings.Contains(l.content, "ERROR boom") {
		t.Fatalf("content = %q", l.content)
	}
	if l.Title() != "logs" {
		t.Errorf("title = %q", l.Title())
	}
}

func TestLogsErrorShownInline(t *testing.T) {
	lg, _ := newLogs(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5, false)
	s, _ := lg.Update(logsLoadedMsg{err: errorString("logs not available for in-progress run")})
	l := s.(*logs)
	if !strings.Contains(l.View(), "not available") {
		t.Fatalf("view = %q", l.View())
	}
}
