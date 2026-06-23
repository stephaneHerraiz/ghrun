package gh

import (
	"encoding/json"
	"fmt"
	"time"
)

// Client provides typed access to GitHub Actions via a Runner.
type Client struct {
	run Runner
}

// NewClient builds a Client over the given Runner.
func NewClient(r Runner) *Client { return &Client{run: r} }

const runListFields = "databaseId,number,workflowName,displayTitle,status,conclusion,headBranch,event,createdAt,startedAt"

// jsonRun mirrors `gh run list --json` output.
type jsonRun struct {
	DatabaseID   int64     `json:"databaseId"`
	Number       int       `json:"number"`
	WorkflowName string    `json:"workflowName"`
	DisplayTitle string    `json:"displayTitle"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HeadBranch   string    `json:"headBranch"`
	Event        string    `json:"event"`
	CreatedAt    time.Time `json:"createdAt"`
	StartedAt    time.Time `json:"startedAt"`
}

func (j jsonRun) toRun() Run {
	return Run{
		ID:           j.DatabaseID,
		Number:       j.Number,
		WorkflowName: j.WorkflowName,
		Title:        j.DisplayTitle,
		Status:       j.Status,
		Conclusion:   j.Conclusion,
		HeadBranch:   j.HeadBranch,
		Event:        j.Event,
		CreatedAt:    j.CreatedAt,
		StartedAt:    j.StartedAt,
	}
}

// ListRuns returns up to limit recent runs for repo.
func (c *Client) ListRuns(repo RepoRef, limit int) ([]Run, error) {
	out, err := c.run.Exec("run", "list", "-R", repo.String(),
		"--limit", fmt.Sprintf("%d", limit), "--json", runListFields)
	if err != nil {
		return nil, err
	}
	var raw []jsonRun
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing run list: %w", err)
	}
	runs := make([]Run, len(raw))
	for i, j := range raw {
		runs[i] = j.toRun()
	}
	return runs, nil
}

type jsonWorkflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// ListWorkflows returns the workflows defined in repo.
func (c *Client) ListWorkflows(repo RepoRef) ([]Workflow, error) {
	out, err := c.run.Exec("workflow", "list", "-R", repo.String(),
		"--json", "name,id,path,state")
	if err != nil {
		return nil, err
	}
	var raw []jsonWorkflow
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing workflow list: %w", err)
	}
	wfs := make([]Workflow, len(raw))
	for i, j := range raw {
		wfs[i] = Workflow{ID: j.ID, Name: j.Name, Path: j.Path, State: j.State}
	}
	return wfs, nil
}
