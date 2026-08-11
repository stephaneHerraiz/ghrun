package gh

import (
	"encoding/json"
	"fmt"
)

// jsonRunMeta mirrors the subset of GET /repos/{o}/{r}/actions/runs/{id} we use.
type jsonRunMeta struct {
	Name       string `json:"name"`
	Number     int    `json:"run_number"`
	Path       string `json:"path"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

// RunMeta returns the run's workflow path, head commit and display metadata.
// It goes through `gh api` rather than `gh run view --json` because the
// workflow *file path* is not among the fields that command can return.
func (c *Client) RunMeta(repo RepoRef, id int64) (RunMeta, error) {
	out, err := c.run.Exec("api",
		fmt.Sprintf("repos/%s/%s/actions/runs/%d", repo.Owner, repo.Name, id))
	if err != nil {
		return RunMeta{}, err
	}
	var raw jsonRunMeta
	if err := json.Unmarshal(out, &raw); err != nil {
		return RunMeta{}, fmt.Errorf("parsing run meta: %w", err)
	}
	return RunMeta{
		WorkflowName: raw.Name,
		WorkflowPath: raw.Path,
		HeadSHA:      raw.HeadSHA,
		HeadBranch:   raw.HeadBranch,
		Number:       raw.Number,
		Status:       raw.Status,
		Conclusion:   raw.Conclusion,
		WebURL:       raw.HTMLURL,
	}, nil
}
