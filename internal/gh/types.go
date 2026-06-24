package gh

import (
	"fmt"
	"strings"
	"time"
)

// RepoRef identifies a repository as owner/name.
type RepoRef struct {
	Owner string
	Name  string
}

func (r RepoRef) String() string { return r.Owner + "/" + r.Name }

// ParseRepoRef parses "owner/name".
func ParseRepoRef(s string) (RepoRef, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RepoRef{}, fmt.Errorf("invalid repo ref %q (want owner/name)", s)
	}
	return RepoRef{Owner: parts[0], Name: parts[1]}, nil
}

// Workflow is a GitHub Actions workflow definition.
type Workflow struct {
	ID    int64
	Name  string
	Path  string
	State string
}

// InputType enumerates workflow_dispatch input types.
type InputType string

const (
	InputString  InputType = "string"
	InputBoolean InputType = "boolean"
	InputChoice  InputType = "choice"
	InputNumber  InputType = "number"
	InputEnv     InputType = "environment"
)

// Input is a single workflow_dispatch input parameter.
type Input struct {
	Name        string
	Description string
	Type        InputType
	Default     string
	Required    bool
	Options     []string // for choice
}

// Run is a workflow run summary.
type Run struct {
	ID           int64
	Number       int
	WorkflowName string
	Title        string
	Status       string // queued | in_progress | completed
	Conclusion   string // success | failure | cancelled | ...
	HeadBranch   string
	Event        string
	CreatedAt    time.Time
	StartedAt    time.Time
}

// Active reports whether the run is queued or running.
func (r Run) Active() bool { return r.Status != "completed" }

// Step is a single step inside a job.
type Step struct {
	Number     int
	Name       string
	Status     string
	Conclusion string
}

// Job is a job inside a run.
type Job struct {
	ID         int64
	Name       string
	Status     string
	Conclusion string
	Steps      []Step
}

// RunDetail is a run plus its jobs.
type RunDetail struct {
	Run
	Jobs []Job
}
