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

func TestRunDetailExplainKeyEmitsMsg(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.explainAvailable = true
	rd.Update(runDetailLoadedMsg{detail: failedDetail()})
	_, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd == nil {
		t.Fatal("'e' on a failed run must emit explainRunMsg")
	}
	msg, ok := cmd().(explainRunMsg)
	if !ok {
		t.Fatalf("msg = %T", cmd())
	}
	if msg.id != 5 || msg.repo.Owner != "o" || msg.detail.WorkflowName != "CI" {
		t.Errorf("msg = %+v", msg)
	}
}

func TestRunDetailExplainKeyInactiveOnSuccess(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.explainAvailable = true
	rd.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 5, Status: "completed", Conclusion: "success"}}})
	if _, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}); cmd != nil {
		t.Error("'e' must be inactive on a successful run")
	}
}

func TestRunDetailExplainKeyInactiveWithoutService(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.Update(runDetailLoadedMsg{detail: failedDetail()})
	if _, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}); cmd != nil {
		t.Error("'e' must be inactive when explain is disabled")
	}
}

func TestRunDetailFooterExplainHint(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.explainAvailable = true
	s, _ := rd.Update(runDetailLoadedMsg{detail: failedDetail()})
	if !strings.Contains(s.View(), "e explain") {
		t.Errorf("failed run must hint 'e explain':\n%s", s.View())
	}
	rd2, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 6)
	rd2.explainAvailable = true
	s2, _ := rd2.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 6, Status: "completed", Conclusion: "success"}}})
	if strings.Contains(s2.View(), "e explain") {
		t.Error("successful run must not hint explain")
	}
	rd3, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 7)
	s3, _ := rd3.Update(runDetailLoadedMsg{detail: failedDetail()})
	if strings.Contains(s3.View(), "e explain") {
		t.Error("disabled explain must not hint")
	}
}
