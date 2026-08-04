package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// fakeExplainService also serves the app-level tests in Task 10.
type fakeExplainService struct {
	local     explain.LocalResult
	localErr  error
	claude    explain.ClaudeResult
	claudeErr error
	askCalls  int
	lastReq   explain.ExplainRequest
}

func (f *fakeExplainService) ResolveLocal(context.Context, string) (explain.LocalResult, error) {
	return f.local, f.localErr
}
func (f *fakeExplainService) AskClaude(_ context.Context, req explain.ExplainRequest) (explain.ClaudeResult, error) {
	f.askCalls++
	f.lastReq = req
	return f.claude, f.claudeErr
}

func failedDetail() gh.RunDetail {
	return gh.RunDetail{
		Run: gh.Run{ID: 5, Status: "completed", Conclusion: "failure", WorkflowName: "CI"},
		Jobs: []gh.Job{{
			Name: "build", Status: "completed", Conclusion: "failure",
			Steps: []gh.Step{
				{Name: "checkout", Status: "completed", Conclusion: "success"},
				{Name: "test", Status: "completed", Conclusion: "failure"},
			},
		}},
	}
}

func newTestExplain(svc ExplainService) *explainScreen {
	e, _ := newExplain(nil, svc, gh.RepoRef{Owner: "o", Name: "r"}, 5, failedDetail())
	return e
}

func TestExplainLogLoadedTriggersLocalSearch(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	s, cmd := e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	if cmd == nil {
		t.Fatal("log loaded must trigger the local search")
	}
	if !strings.Contains(s.View(), "Searching local knowledge") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainLogErrorShown(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	s, _ := e.Update(explainLogLoadedMsg{id: 5, err: errorString("no logs available")})
	if !strings.Contains(s.View(), "no logs available") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainLocalHitShowsBadgeAndText(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	s, cmd := e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{
		Found: true, Explanation: "Root cause: OOM.", Similarity: 0.92,
	}})
	if cmd != nil {
		t.Error("a hit must not ask claude")
	}
	v := s.View()
	if !strings.Contains(v, "Root cause: OOM.") || !strings.Contains(v, "🧠 local · 92%") {
		t.Errorf("view = %q", v)
	}
}

func TestExplainExactHitBadge(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	s, _ := e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{Found: true, Exact: true, Explanation: "x", Similarity: 1}})
	if !strings.Contains(s.View(), "🧠 local · exact") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainMissAsksClaude(t *testing.T) {
	svc := &fakeExplainService{claude: explain.ClaudeResult{Explanation: "fresh", Source: "claude-sonnet-5"}}
	e := newTestExplain(svc)
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	s, cmd := e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{}})
	if cmd == nil {
		t.Fatal("a miss must ask claude")
	}
	if !strings.Contains(s.View(), "Asking claude") {
		t.Errorf("view = %q", s.View())
	}
	msg := cmd() // runs askClaudeCmd against the fake
	if svc.askCalls != 1 {
		t.Fatalf("askCalls = %d", svc.askCalls)
	}
	if svc.lastReq.Repo != "o/r" || svc.lastReq.Workflow != "CI" ||
		len(svc.lastReq.FailedSteps) != 1 || svc.lastReq.FailedSteps[0] != "build / test" {
		t.Errorf("req = %+v", svc.lastReq)
	}
	s, _ = s.Update(msg)
	v := s.View()
	if !strings.Contains(v, "fresh") || !strings.Contains(v, "✨ claude-sonnet-5") {
		t.Errorf("view = %q", v)
	}
}

func TestExplainRAGDisabledShowsRealReason(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	s, cmd := e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{
		RAGDisabled:    true,
		DisabledReason: "ollama: 500: input length exceeds the context length",
	}})
	if cmd == nil {
		t.Fatal("RAG disabled must still ask claude")
	}
	v := s.View()
	// The real cause must be shown, and the old hardcoded "Ollama unreachable"
	// lie must be gone — Ollama answered fine, the input was just too long.
	if !strings.Contains(v, "input length exceeds the context length") {
		t.Errorf("view must show the real reason: %q", v)
	}
	if strings.Contains(v, "unreachable") {
		t.Errorf("must not claim Ollama is unreachable: %q", v)
	}
}

func TestExplainClaudeErrorFallsBackToBestGuess(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{Explanation: "old similar failure", Similarity: 0.55}})
	s, _ := e.Update(explainClaudeMsg{id: 5, err: errorString("all explainers down")})
	v := s.View()
	if !strings.Contains(v, "old similar failure") || !strings.Contains(v, "best guess · 55%") {
		t.Errorf("view = %q", v)
	}
}

func TestExplainClaudeErrorWithoutGuess(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{}})
	s, _ := e.Update(explainClaudeMsg{id: 5, err: errorString("all explainers down")})
	v := s.View()
	if !strings.Contains(v, "all explainers down") || !strings.Contains(v, "l") {
		t.Errorf("view must show the error and point at raw logs: %q", v)
	}
}

func TestExplainRegenerateKey(t *testing.T) {
	svc := &fakeExplainService{claude: explain.ClaudeResult{Explanation: "v2", Source: "claude-sonnet-5"}}
	e := newTestExplain(svc)
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{Found: true, Explanation: "v1", Similarity: 0.9}})
	s, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("'r' must regenerate")
	}
	if !strings.Contains(s.View(), "Asking claude") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainLogsKeyPushesLogs(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil {
		t.Fatal("'l' must push the logs screen")
	}
	if e.Title() != "explain" {
		t.Errorf("title = %q", e.Title())
	}
}

func TestExplainRegenerateIgnoredWhileSearching(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"}) // -> phaseSearching
	s, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd != nil {
		t.Error("'r' must be ignored while the local search is in flight")
	}
	if !strings.Contains(s.View(), "Searching local knowledge") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainStaleLocalResultIgnored(t *testing.T) {
	svc := &fakeExplainService{claude: explain.ClaudeResult{Explanation: "fresh", Source: "claude-sonnet-5"}}
	e := newTestExplain(svc)
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{}}) // miss -> phaseAsking
	// A duplicate/stale local result must not clobber the asking state.
	s, cmd := e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{Found: true, Explanation: "stale", Similarity: 0.99}})
	if cmd != nil {
		t.Error("stale local result must not trigger anything")
	}
	if !strings.Contains(s.View(), "Asking claude") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainRegenerateClearsStaleWarning(t *testing.T) {
	svc := &fakeExplainService{claude: explain.ClaudeResult{Explanation: "v2", Source: "claude-sonnet-5"}}
	e := newTestExplain(svc)
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{Explanation: "old guess", Similarity: 0.5}})
	e.Update(explainClaudeMsg{id: 5, err: errorString("api down")}) // best guess + warning
	s, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("'r' from best-guess state must regenerate")
	}
	s, _ = s.Update(cmd())
	v := s.View()
	if !strings.Contains(v, "v2") || strings.Contains(v, "api down") {
		t.Errorf("stale warning survived regenerate: %q", v)
	}
}

func TestExplainForeignRunMsgDropped(t *testing.T) {
	svc := &fakeExplainService{claude: explain.ClaudeResult{Explanation: "fresh", Source: "claude-sonnet-5"}}
	e := newTestExplain(svc)
	e.Update(explainLogLoadedMsg{id: 5, text: "Error: boom"})
	e.Update(explainLocalMsg{id: 5, res: explain.LocalResult{}}) // miss -> asking
	s, cmd := e.Update(explainClaudeMsg{id: 99, res: explain.ClaudeResult{Explanation: "other run's answer", Source: "x"}})
	if cmd != nil {
		t.Error("foreign-run message must be inert")
	}
	v := s.View()
	if strings.Contains(v, "other run's answer") || !strings.Contains(v, "Asking claude") {
		t.Errorf("view = %q", v)
	}
}
