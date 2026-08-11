package ui

import (
	"context"
	"time"

	"github.com/stephaneHerraiz/ghrun/internal/explain"
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
	RunMeta(repo gh.RepoRef, id int64) (gh.RunMeta, error)
	WorkflowFile(repo gh.RepoRef, path, ref string) ([]byte, error)
	OpenWeb(repo gh.RepoRef, id int64) error
	ListOrgRepos(org string) ([]gh.RepoRef, error)
	ListNamespaces() ([]string, error)
}

// ExplainService is the subset of *explain.Service the UI depends on (for
// mockability, like GHClient).
type ExplainService interface {
	ResolveLocal(ctx context.Context, logText string) (explain.LocalResult, error)
	AskClaude(ctx context.Context, req explain.ExplainRequest) (explain.ClaudeResult, error)
}
