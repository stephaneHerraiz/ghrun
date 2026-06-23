package gh

import (
	"testing"
	"time"
)

func argvContains(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}

func TestDispatchWorkflowBuildsArgs(t *testing.T) {
	f := (&fakeRunner{}).push("", nil)
	c := NewClient(f)
	err := c.DispatchWorkflow(RepoRef{"o", "r"}, 1234, "main",
		map[string]string{"version": "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	got := f.lastCall()
	if got[0] != "workflow" || got[1] != "run" || got[2] != "1234" {
		t.Fatalf("argv = %v", got)
	}
	if !argvContains(got, "--ref") || !argvContains(got, "main") {
		t.Errorf("missing ref: %v", got)
	}
	if !argvContains(got, "-f") || !argvContains(got, "version=1.2.3") {
		t.Errorf("missing input: %v", got)
	}
}

func TestFindRunSincePicksNewestAfter(t *testing.T) {
	since := time.Date(2026, 6, 23, 8, 0, 0, 0, time.UTC)
	const out = `[
	  {"databaseId":200,"number":2,"workflowName":"D","displayTitle":"","status":"queued","conclusion":"","headBranch":"main","event":"workflow_dispatch","createdAt":"2026-06-23T08:00:03Z","startedAt":"0001-01-01T00:00:00Z"},
	  {"databaseId":199,"number":1,"workflowName":"D","displayTitle":"","status":"completed","conclusion":"success","headBranch":"main","event":"push","createdAt":"2026-06-23T07:00:00Z","startedAt":"2026-06-23T07:00:01Z"}
	]`
	f := (&fakeRunner{}).push(out, nil)
	c := NewClient(f)
	id, err := c.FindRunSince(RepoRef{"o", "r"}, 1234, since)
	if err != nil {
		t.Fatal(err)
	}
	if id != 200 {
		t.Fatalf("id = %d, want 200", id)
	}
	got := f.lastCall()
	if !argvContains(got, "--workflow") || !argvContains(got, "1234") {
		t.Errorf("missing workflow filter: %v", got)
	}
}

func TestFindRunSinceNoneYet(t *testing.T) {
	since := time.Date(2026, 6, 23, 8, 0, 0, 0, time.UTC)
	const out = `[{"databaseId":199,"number":1,"workflowName":"D","displayTitle":"","status":"completed","conclusion":"success","headBranch":"main","event":"push","createdAt":"2026-06-23T07:00:00Z","startedAt":"2026-06-23T07:00:01Z"}]`
	f := (&fakeRunner{}).push(out, nil)
	c := NewClient(f)
	id, err := c.FindRunSince(RepoRef{"o", "r"}, 1234, since)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0", id)
	}
}

func TestRerunFailedFlag(t *testing.T) {
	f := (&fakeRunner{}).push("", nil)
	c := NewClient(f)
	if err := c.Rerun(RepoRef{"o", "r"}, 5, true); err != nil {
		t.Fatal(err)
	}
	if got := f.lastCall(); !argvContains(got, "--failed") {
		t.Errorf("argv = %v, want --failed", got)
	}
}

func TestRunLogsFailedFlag(t *testing.T) {
	f := (&fakeRunner{}).push("log output", nil)
	c := NewClient(f)
	s, err := c.RunLogs(RepoRef{"o", "r"}, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if s != "log output" {
		t.Errorf("logs = %q", s)
	}
	if got := f.lastCall(); !argvContains(got, "--log-failed") {
		t.Errorf("argv = %v, want --log-failed", got)
	}
}
