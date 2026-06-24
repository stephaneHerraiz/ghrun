package ui

import (
	"testing"

	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

func TestAggregateRepoStatus(t *testing.T) {
	repo := gh.RepoRef{Owner: "o", Name: "r"}
	runs := []gh.Run{
		{ID: 3, Status: "in_progress", WorkflowName: "CI"},
		{ID: 2, Status: "completed", Conclusion: "success", WorkflowName: "Deploy"},
		{ID: 1, Status: "queued", WorkflowName: "Nightly"},
	}
	st := aggregateRepoStatus(repo, runs)
	if st.latest == nil || st.latest.ID != 3 {
		t.Fatalf("latest = %+v, want id 3", st.latest)
	}
	if st.active != 2 {
		t.Errorf("active = %d, want 2", st.active)
	}
}

func TestAggregateRepoStatusEmpty(t *testing.T) {
	st := aggregateRepoStatus(gh.RepoRef{Owner: "o", Name: "r"}, nil)
	if st.latest != nil || st.active != 0 {
		t.Errorf("empty repo status = %+v", st)
	}
}
