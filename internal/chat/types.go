// Package chat materialises the context of a GitHub Actions run on disk and
// builds the command that hands the terminal over to the interactive claude
// CLI. It has no Bubbletea and no gh dependency, so every part is testable
// without a TTY, a network or a real binary.
package chat

import "errors"

// ErrNoContext means neither the log nor the workflow file could be fetched,
// so there would be nothing for Claude to look at.
var ErrNoContext = errors.New("neither the run log nor the workflow file could be fetched")

// Context is what the UI knows about the run being discussed.
type Context struct {
	Repo         string // "owner/name"
	RunID        int64
	RunNumber    int
	Workflow     string // "Build & test"
	WorkflowPath string // ".github/workflows/ci.yaml"
	Branch       string
	HeadSHA      string
	Status       string
	Conclusion   string
	FailedSteps  []string // "job / step"
	WebURL       string
}

// Failed reports whether the run's conclusion warrants a debugging angle. It
// also decides which log the caller fetches (--log-failed vs --log).
func (c Context) Failed() bool {
	return c.Conclusion == "failure" || c.Conclusion == "timed_out"
}

// Session is a Context materialised on disk, ready to hand to claude. The
// file fields are absolute paths, or empty when that piece could not be
// fetched — Prompt renders a different line in that case.
type Session struct {
	Context
	Dir          string // the context directory
	LogFile      string
	WorkflowFile string
	CloneDir     string // the repo's local clone, empty when none was found
}
