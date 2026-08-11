package gh

import (
	"errors"
	"testing"
)

const sampleRunMetaJSON = `{
  "name": "Build & test",
  "run_number": 1419,
  "path": ".github/workflows/ci.yaml",
  "head_sha": "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0",
  "head_branch": "feature/x",
  "status": "completed",
  "conclusion": "failure",
  "html_url": "https://github.com/o/r/actions/runs/4211"
}`

func TestRunMetaParses(t *testing.T) {
	f := (&fakeRunner{}).push(sampleRunMetaJSON, nil)
	c := NewClient(f)

	m, err := c.RunMeta(RepoRef{"o", "r"}, 4211)
	if err != nil {
		t.Fatal(err)
	}
	if m.WorkflowName != "Build & test" || m.WorkflowPath != ".github/workflows/ci.yaml" {
		t.Errorf("workflow = %q / %q", m.WorkflowName, m.WorkflowPath)
	}
	if m.HeadSHA != "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0" || m.HeadBranch != "feature/x" {
		t.Errorf("head = %q / %q", m.HeadSHA, m.HeadBranch)
	}
	if m.Number != 1419 || m.Status != "completed" || m.Conclusion != "failure" {
		t.Errorf("meta = %+v", m)
	}
	if m.WebURL != "https://github.com/o/r/actions/runs/4211" {
		t.Errorf("WebURL = %q", m.WebURL)
	}

	got := f.lastCall()
	if len(got) != 2 || got[0] != "api" || got[1] != "repos/o/r/actions/runs/4211" {
		t.Errorf("argv = %v", got)
	}
}

func TestRunMetaPropagatesRunnerError(t *testing.T) {
	boom := errors.New("boom")
	c := NewClient((&fakeRunner{}).push("", boom))
	if _, err := c.RunMeta(RepoRef{"o", "r"}, 1); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestRunMetaRejectsGarbage(t *testing.T) {
	c := NewClient((&fakeRunner{}).push("not json", nil))
	if _, err := c.RunMeta(RepoRef{"o", "r"}, 1); err == nil {
		t.Fatal("expected a parse error")
	}
}
