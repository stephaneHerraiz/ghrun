package ui

import (
	"time"

	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// GHClient is the subset of *gh.Client the UI depends on (for mockability).
type GHClient interface {
	ListRuns(repo gh.RepoRef, limit int) ([]gh.Run, error)
	GetRun(repo gh.RepoRef, id int64) (gh.RunDetail, error)
	ListWorkflows(repo gh.RepoRef) ([]gh.Workflow, error)
	WorkflowInputs(repo gh.RepoRef, path string) ([]gh.Input, error)
	ListBranches(repo gh.RepoRef) ([]string, error)
	DispatchWorkflow(repo gh.RepoRef, workflowID int64, ref string, inputs map[string]string) error
	FindRunSince(repo gh.RepoRef, workflowID int64, since time.Time) (int64, error)
	Rerun(repo gh.RepoRef, id int64, failedOnly bool) error
	Cancel(repo gh.RepoRef, id int64) error
	RunLogs(repo gh.RepoRef, id int64, failedOnly bool) (string, error)
	OpenWeb(repo gh.RepoRef, id int64) error
	ListOrgRepos(org string) ([]gh.RepoRef, error)
	ListNamespaces() ([]string, error)
}
