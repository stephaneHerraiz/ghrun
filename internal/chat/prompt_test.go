package chat

import (
	"strings"
	"testing"
)

// failedSession is the nominal case: failed run, both files, clone found.
func failedSession() Session {
	return Session{
		Context:      sampleContext(),
		Dir:          "/cache/acme/widgets/4211",
		LogFile:      "/cache/acme/widgets/4211/log.txt",
		WorkflowFile: "/cache/acme/widgets/4211/workflow.yml",
		CloneDir:     "/home/me/dev/widgets",
	}
}

func TestPromptFailedRun(t *testing.T) {
	p := Prompt(failedSession())
	for _, want := range []string{
		"GitHub Actions run #17 of acme/widgets — failure",
		"Workflow: Build & test (.github/workflows/ci.yaml)",
		"Branch: feature/x · commit df53ccd · https://github.com/acme/widgets/actions/runs/4211",
		"Failed steps: build / npm ci",
		"/cache/acme/widgets/4211/log.txt",
		"/cache/acme/widgets/4211/workflow.yml",
		"as it was at df53ccd",
		"You are in a local clone of this repository",
		"gh run view 4211 --repo acme/widgets --log",
		"Analyse the root cause of this failure, then propose a fix.",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}

func TestPromptSuccessfulRunAsksForASummary(t *testing.T) {
	s := failedSession()
	s.Conclusion = "success"
	s.FailedSteps = nil
	p := Prompt(s)
	if !strings.Contains(p, "— success") {
		t.Errorf("state line wrong:\n%s", p)
	}
	if strings.Contains(p, "Failed steps:") {
		t.Errorf("no failed steps means no such line:\n%s", p)
	}
	if !strings.Contains(p, "Summarise what this run did.") {
		t.Errorf("expected the summary question:\n%s", p)
	}
	if strings.Contains(p, "root cause") {
		t.Errorf("a successful run must not ask for a root cause:\n%s", p)
	}
}

func TestPromptWithoutCloneOmitsTheCloneLine(t *testing.T) {
	s := failedSession()
	s.CloneDir = ""
	if strings.Contains(Prompt(s), "local clone") {
		t.Errorf("no clone means no clone line:\n%s", Prompt(s))
	}
}

func TestPromptWithoutLogSaysSo(t *testing.T) {
	s := failedSession()
	s.LogFile = ""
	p := Prompt(s)
	if strings.Contains(p, "log.txt") {
		t.Errorf("must not reference a log that was not written:\n%s", p)
	}
	if !strings.Contains(p, "no log could be fetched") {
		t.Errorf("expected the missing-log notice:\n%s", p)
	}
}

func TestPromptWithoutWorkflowSaysSo(t *testing.T) {
	s := failedSession()
	s.WorkflowFile = ""
	p := Prompt(s)
	if strings.Contains(p, "workflow.yml") {
		t.Errorf("must not reference a workflow file that was not written:\n%s", p)
	}
	if !strings.Contains(p, "workflow file could not be fetched") {
		t.Errorf("expected the missing-workflow notice:\n%s", p)
	}
}

func TestPromptFallsBackToStatusWhenNotConcluded(t *testing.T) {
	s := failedSession()
	s.Status, s.Conclusion = "in_progress", ""
	if !strings.Contains(Prompt(s), "— in_progress") {
		t.Errorf("expected the status as the state:\n%s", Prompt(s))
	}
}

func TestPromptKeepsAShortSHAAsIs(t *testing.T) {
	s := failedSession()
	s.HeadSHA = "abc"
	if !strings.Contains(Prompt(s), "commit abc") {
		t.Errorf("a short sha must be printed as-is:\n%s", Prompt(s))
	}
}

func TestPromptKeepsTheURLWhenTheBranchIsUnknown(t *testing.T) {
	s := failedSession()
	s.Branch, s.HeadSHA = "", ""
	p := Prompt(s)
	if !strings.Contains(p, "URL: https://github.com/acme/widgets/actions/runs/4211") {
		t.Errorf("the run URL must survive a missing branch:\n%s", p)
	}
	if strings.Contains(p, "Branch:") {
		t.Errorf("no branch means no branch line:\n%s", p)
	}
}
