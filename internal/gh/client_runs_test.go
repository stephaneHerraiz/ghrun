package gh

import "testing"

func TestListRunsParses(t *testing.T) {
	const out = `[
	  {"databaseId":101,"number":7,"workflowName":"CI","displayTitle":"fix: x",
	   "status":"in_progress","conclusion":"","headBranch":"main","event":"push",
	   "createdAt":"2026-06-23T08:00:00Z","startedAt":"2026-06-23T08:00:05Z"},
	  {"databaseId":100,"number":6,"workflowName":"Deploy","displayTitle":"deploy prod",
	   "status":"completed","conclusion":"success","headBranch":"main","event":"workflow_dispatch",
	   "createdAt":"2026-06-23T07:00:00Z","startedAt":"2026-06-23T07:00:05Z"}
	]`
	f := (&fakeRunner{}).push(out, nil)
	c := NewClient(f)

	runs, err := c.ListRuns(RepoRef{"o", "r"}, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs", len(runs))
	}
	if runs[0].ID != 101 || runs[0].WorkflowName != "CI" || !runs[0].Active() {
		t.Errorf("run0 = %+v", runs[0])
	}
	if runs[1].Conclusion != "success" {
		t.Errorf("run1 conclusion = %q", runs[1].Conclusion)
	}
	// Verify the command shape.
	got := f.lastCall()
	want := []string{"run", "list", "-R", "o/r", "--limit", "30", "--json",
		"databaseId,number,workflowName,displayTitle,status,conclusion,headBranch,event,createdAt,startedAt"}
	if len(got) != len(want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
