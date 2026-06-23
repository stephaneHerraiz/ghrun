package gh

import (
	"encoding/json"
	"fmt"
)

type jsonStep struct {
	Number     int    `json:"number"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type jsonJob struct {
	DatabaseID int64      `json:"databaseId"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Conclusion string     `json:"conclusion"`
	Steps      []jsonStep `json:"steps"`
}

type jsonRunDetail struct {
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Jobs       []jsonJob `json:"jobs"`
}

// GetRun returns run status plus its jobs and steps.
func (c *Client) GetRun(repo RepoRef, id int64) (RunDetail, error) {
	out, err := c.run.Exec("run", "view", fmt.Sprintf("%d", id),
		"-R", repo.String(), "--json", "status,conclusion,jobs")
	if err != nil {
		return RunDetail{}, err
	}
	var raw jsonRunDetail
	if err := json.Unmarshal(out, &raw); err != nil {
		return RunDetail{}, fmt.Errorf("parsing run detail: %w", err)
	}
	rd := RunDetail{Run: Run{ID: id, Status: raw.Status, Conclusion: raw.Conclusion}}
	for _, j := range raw.Jobs {
		job := Job{ID: j.DatabaseID, Name: j.Name, Status: j.Status, Conclusion: j.Conclusion}
		for _, s := range j.Steps {
			job.Steps = append(job.Steps, Step{Number: s.Number, Name: s.Name, Status: s.Status, Conclusion: s.Conclusion})
		}
		rd.Jobs = append(rd.Jobs, job)
	}
	return rd, nil
}
