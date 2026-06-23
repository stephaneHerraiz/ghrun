package gh

import (
	"encoding/json"
	"fmt"
	"time"
)

// DispatchWorkflow triggers a workflow_dispatch on ref with the given inputs.
func (c *Client) DispatchWorkflow(repo RepoRef, workflowID int64, ref string, inputs map[string]string) error {
	args := []string{"workflow", "run", fmt.Sprintf("%d", workflowID),
		"-R", repo.String(), "--ref", ref}
	for k, v := range inputs {
		args = append(args, "-f", fmt.Sprintf("%s=%s", k, v))
	}
	_, err := c.run.Exec(args...)
	return err
}

// findRunLimit bounds how many recent runs FindRunSince scans. It only needs
// to cover runs of a single workflow created in the seconds after a dispatch,
// so a small window with headroom is enough.
const findRunLimit = 20

// FindRunSince returns the newest run id for workflowID created at/after since, or 0.
func (c *Client) FindRunSince(repo RepoRef, workflowID int64, since time.Time) (int64, error) {
	out, err := c.run.Exec("run", "list", "-R", repo.String(),
		"--workflow", fmt.Sprintf("%d", workflowID),
		"--limit", fmt.Sprintf("%d", findRunLimit), "--json", "databaseId,createdAt")
	if err != nil {
		return 0, err
	}
	var raw []struct {
		DatabaseID int64     `json:"databaseId"`
		CreatedAt  time.Time `json:"createdAt"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return 0, fmt.Errorf("parsing run list: %w", err)
	}
	var bestID int64
	var bestTime time.Time
	for _, r := range raw {
		if r.CreatedAt.Before(since) {
			continue
		}
		if r.CreatedAt.After(bestTime) {
			bestTime = r.CreatedAt
			bestID = r.DatabaseID
		}
	}
	return bestID, nil
}

// Rerun re-runs a finished run; failedOnly re-runs only failed jobs.
func (c *Client) Rerun(repo RepoRef, id int64, failedOnly bool) error {
	args := []string{"run", "rerun", fmt.Sprintf("%d", id), "-R", repo.String()}
	if failedOnly {
		args = append(args, "--failed")
	}
	_, err := c.run.Exec(args...)
	return err
}

// Cancel cancels an in-progress run.
func (c *Client) Cancel(repo RepoRef, id int64) error {
	_, err := c.run.Exec("run", "cancel", fmt.Sprintf("%d", id), "-R", repo.String())
	return err
}

// RunLogs returns the run's logs; failedOnly limits to failed steps.
func (c *Client) RunLogs(repo RepoRef, id int64, failedOnly bool) (string, error) {
	logFlag := "--log"
	if failedOnly {
		logFlag = "--log-failed"
	}
	out, err := c.run.Exec("run", "view", fmt.Sprintf("%d", id), "-R", repo.String(), logFlag)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// OpenWeb opens the run in a browser (gh handles launching the browser).
func (c *Client) OpenWeb(repo RepoRef, id int64) error {
	_, err := c.run.Exec("run", "view", fmt.Sprintf("%d", id), "-R", repo.String(), "--web")
	return err
}

// AuthStatus returns an error if the user is not authenticated with gh.
func (c *Client) AuthStatus() error {
	_, err := c.run.Exec("auth", "status")
	return err
}
