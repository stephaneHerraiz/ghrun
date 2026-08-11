package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// fakeChatFetcher is the narrow client prepareChatCmd needs.
type fakeChatFetcher struct {
	meta    gh.RunMeta
	metaErr error
	wf      []byte
	wfErr   error
	// logs is keyed by failedOnly.
	logs    map[bool]string
	logErr  error
	wfCalls [][2]string // path, ref
}

func (f *fakeChatFetcher) RunMeta(gh.RepoRef, int64) (gh.RunMeta, error) {
	return f.meta, f.metaErr
}

func (f *fakeChatFetcher) WorkflowFile(_ gh.RepoRef, path, ref string) ([]byte, error) {
	f.wfCalls = append(f.wfCalls, [2]string{path, ref})
	return f.wf, f.wfErr
}

func (f *fakeChatFetcher) RunLogs(_ gh.RepoRef, _ int64, failedOnly bool) (string, error) {
	if f.logErr != nil {
		return "", f.logErr
	}
	return f.logs[failedOnly], nil
}

func okFetcher() *fakeChatFetcher {
	return &fakeChatFetcher{
		meta: gh.RunMeta{
			WorkflowName: "Build & test",
			WorkflowPath: ".github/workflows/ci.yaml",
			HeadSHA:      "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0",
			HeadBranch:   "feature/x",
			Number:       17,
			Status:       "completed",
			Conclusion:   "failure",
			WebURL:       "https://github.com/acme/widgets/actions/runs/4211",
		},
		wf:   []byte("name: CI\n"),
		logs: map[bool]string{true: "npm ERR! boom"},
	}
}

func failedRunDetail() gh.RunDetail {
	return gh.RunDetail{
		Run: gh.Run{ID: 4211, Status: "completed", Conclusion: "failure"},
		Jobs: []gh.Job{{
			Name: "build", Conclusion: "failure",
			Steps: []gh.Step{
				{Name: "checkout", Conclusion: "success"},
				{Name: "npm ci", Conclusion: "failure"},
			},
		}},
	}
}

// chatCfg points claudeCmd at a binary guaranteed to be on PATH so the
// LookPath guard passes without a real claude install.
func chatCfg() config.ChatConfig {
	return config.ChatConfig{ClaudeCmd: "go", CloneRoots: nil}
}

func repoRef() gh.RepoRef { return gh.RepoRef{Owner: "acme", Name: "widgets"} }

func TestPrepareChatBuildsTheCommand(t *testing.T) {
	base := t.TempDir()
	f := okFetcher()
	msg := prepareChatCmd(f, chatCfg(), base, repoRef(), 4211, failedRunDetail())()

	ready, ok := msg.(chatReadyMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	prompt := ready.cmd.Args[len(ready.cmd.Args)-1]
	for _, want := range []string{
		"run #17 of acme/widgets — failure",
		"Build & test (.github/workflows/ci.yaml)",
		"Failed steps: build / npm ci",
		"Analyse the root cause",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	// The workflow was fetched at the run's commit, not at HEAD.
	if len(f.wfCalls) != 1 || f.wfCalls[0][1] != f.meta.HeadSHA {
		t.Errorf("workflow calls = %v", f.wfCalls)
	}
	// Both files landed on disk.
	dir := filepath.Join(base, "acme", "widgets", "4211")
	if _, err := os.Stat(filepath.Join(dir, "log.txt")); err != nil {
		t.Errorf("log.txt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "workflow.yml")); err != nil {
		t.Errorf("workflow.yml: %v", err)
	}
}

func TestPrepareChatUsesFullLogForASuccessfulRun(t *testing.T) {
	f := okFetcher()
	f.meta.Conclusion = "success"
	f.logs = map[bool]string{false: "everything fine"}
	detail := failedRunDetail()
	detail.Conclusion = "success"
	detail.Jobs[0].Steps[1].Conclusion = "success"

	msg := prepareChatCmd(f, chatCfg(), t.TempDir(), repoRef(), 4211, detail)()
	ready, ok := msg.(chatReadyMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	prompt := ready.cmd.Args[len(ready.cmd.Args)-1]
	if !strings.Contains(prompt, "Summarise what this run did.") {
		t.Errorf("expected the summary question:\n%s", prompt)
	}
}

func TestPrepareChatFallsBackToTheFullLogWhenFailedOnlyIsEmpty(t *testing.T) {
	base := t.TempDir()
	f := okFetcher()
	f.logs = map[bool]string{true: "   ", false: "the whole log"}
	msg := prepareChatCmd(f, chatCfg(), base, repoRef(), 4211, failedRunDetail())()
	if _, ok := msg.(chatReadyMsg); !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	b, err := os.ReadFile(filepath.Join(base, "acme", "widgets", "4211", "log.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "the whole log" {
		t.Errorf("log = %q, want the full-log fallback", b)
	}
}

func TestPrepareChatSurvivesMissingMeta(t *testing.T) {
	f := okFetcher()
	f.metaErr = errors.New("api down")
	msg := prepareChatCmd(f, chatCfg(), t.TempDir(), repoRef(), 4211, failedRunDetail())()
	ready, ok := msg.(chatReadyMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	prompt := ready.cmd.Args[len(ready.cmd.Args)-1]
	if !strings.Contains(prompt, "workflow file could not be fetched") {
		t.Errorf("expected the missing-workflow notice:\n%s", prompt)
	}
	// A web URL is still built from the repo and id.
	if !strings.Contains(prompt, "https://github.com/acme/widgets/actions/runs/4211") {
		t.Errorf("expected a fallback web url:\n%s", prompt)
	}
	if len(f.wfCalls) != 0 {
		t.Errorf("no workflow path means no contents call, got %v", f.wfCalls)
	}
}

func TestPrepareChatErrorsWhenNothingCanBeFetched(t *testing.T) {
	f := &fakeChatFetcher{metaErr: errors.New("nope"), logErr: errors.New("nope")}
	msg := prepareChatCmd(f, chatCfg(), t.TempDir(), repoRef(), 4211, failedRunDetail())()
	em, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	if !strings.Contains(em.err.Error(), "chat") {
		t.Errorf("err = %v", em.err)
	}
}

func TestPrepareChatErrorsWhenClaudeIsNotInstalled(t *testing.T) {
	cfg := config.ChatConfig{ClaudeCmd: "definitely-not-a-real-binary-xyz"}
	msg := prepareChatCmd(okFetcher(), cfg, t.TempDir(), repoRef(), 4211, failedRunDetail())()
	em, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	if !strings.Contains(em.err.Error(), "definitely-not-a-real-binary-xyz") {
		t.Errorf("err = %v", em.err)
	}
}
