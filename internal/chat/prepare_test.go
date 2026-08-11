package chat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sampleContext() Context {
	return Context{
		Repo:         "acme/widgets",
		RunID:        4211,
		RunNumber:    17,
		Workflow:     "Build & test",
		WorkflowPath: ".github/workflows/ci.yaml",
		Branch:       "feature/x",
		HeadSHA:      "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0",
		Status:       "completed",
		Conclusion:   "failure",
		FailedSteps:  []string{"build / npm ci"},
		WebURL:       "https://github.com/acme/widgets/actions/runs/4211",
	}
}

func TestPrepareWritesBothFiles(t *testing.T) {
	base := t.TempDir()
	s, err := Prepare(base, sampleContext(), []byte("log body"), []byte("name: CI\n"))
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(base, "acme", "widgets", "4211")
	if s.Dir != wantDir {
		t.Errorf("Dir = %q, want %q", s.Dir, wantDir)
	}
	if s.LogFile != filepath.Join(wantDir, "log.txt") {
		t.Errorf("LogFile = %q", s.LogFile)
	}
	if s.WorkflowFile != filepath.Join(wantDir, "workflow.yml") {
		t.Errorf("WorkflowFile = %q", s.WorkflowFile)
	}
	if b, err := os.ReadFile(s.LogFile); err != nil || string(b) != "log body" {
		t.Errorf("log content = %q, err %v", b, err)
	}
	if b, err := os.ReadFile(s.WorkflowFile); err != nil || string(b) != "name: CI\n" {
		t.Errorf("workflow content = %q, err %v", b, err)
	}
	// The Context travels with the Session.
	if s.Repo != "acme/widgets" || s.RunID != 4211 {
		t.Errorf("context not carried: %+v", s.Context)
	}
}

func TestPrepareSkipsEmptyLog(t *testing.T) {
	base := t.TempDir()
	s, err := Prepare(base, sampleContext(), nil, []byte("name: CI\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.LogFile != "" {
		t.Errorf("LogFile = %q, want empty", s.LogFile)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "log.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("log.txt must not be created when there is no log")
	}
	if s.WorkflowFile == "" {
		t.Error("the workflow file should still have been written")
	}
}

func TestPrepareSkipsEmptyWorkflow(t *testing.T) {
	base := t.TempDir()
	s, err := Prepare(base, sampleContext(), []byte("log body"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.WorkflowFile != "" {
		t.Errorf("WorkflowFile = %q, want empty", s.WorkflowFile)
	}
	if s.LogFile == "" {
		t.Error("the log should still have been written")
	}
}

func TestPrepareRejectsEmptyContext(t *testing.T) {
	base := t.TempDir()
	_, err := Prepare(base, sampleContext(), nil, nil)
	if !errors.Is(err, ErrNoContext) {
		t.Fatalf("err = %v, want ErrNoContext", err)
	}
	// Nothing was created on disk.
	if _, err := os.Stat(filepath.Join(base, "acme")); !errors.Is(err, os.ErrNotExist) {
		t.Error("Prepare must not create directories when there is no context")
	}
}

func TestPrepareClearsStaleFiles(t *testing.T) {
	base := t.TempDir()
	if _, err := Prepare(base, sampleContext(), []byte("old log"), []byte("old wf")); err != nil {
		t.Fatal(err)
	}
	// Second run: the workflow could not be fetched this time.
	s, err := Prepare(base, sampleContext(), []byte("new log"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "workflow.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a stale workflow.yml from a previous run must be cleared")
	}
	if b, _ := os.ReadFile(s.LogFile); string(b) != "new log" {
		t.Errorf("log = %q, want the fresh one", b)
	}
}

func TestContextFailed(t *testing.T) {
	for _, tc := range []struct {
		conclusion string
		want       bool
	}{
		{"failure", true},
		{"timed_out", true},
		{"success", false},
		{"cancelled", false},
		{"", false},
	} {
		if got := (Context{Conclusion: tc.conclusion}).Failed(); got != tc.want {
			t.Errorf("Failed(%q) = %v, want %v", tc.conclusion, got, tc.want)
		}
	}
}
