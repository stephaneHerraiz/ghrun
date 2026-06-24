package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestRunDetailRendersJobsAndSteps(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	s, _ := rd.Update(runDetailLoadedMsg{detail: gh.RunDetail{
		Run: gh.Run{ID: 5, Status: "completed", Conclusion: "failure"},
		Jobs: []gh.Job{{
			Name: "build", Status: "completed", Conclusion: "failure",
			Steps: []gh.Step{
				{Number: 1, Name: "checkout", Status: "completed", Conclusion: "success"},
				{Number: 2, Name: "test", Status: "completed", Conclusion: "failure"},
			},
		}},
	}})
	d := s.(*rundetail)
	view := d.View()
	if !strings.Contains(view, "build") || !strings.Contains(view, "test") {
		t.Fatalf("view missing jobs/steps:\n%s", view)
	}
}

func TestRunDetailLogsKeyPushesLogs(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 5, Status: "completed", Conclusion: "failure"}}})
	_, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil {
		t.Fatal("'l' should push the logs screen")
	}
}
