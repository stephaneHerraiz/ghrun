package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	logFileName      = "log.txt"
	workflowFileName = "workflow.yml"
)

// Prepare writes the run's context under baseDir and returns the Session
// describing what actually landed on disk. Empty contents are not written and
// leave their Session field empty. The run's directory is recreated from
// scratch so a file left over by a previous attempt can never be presented as
// current.
func Prepare(baseDir string, ctx Context, log, workflowYAML []byte) (Session, error) {
	if len(log) == 0 && len(workflowYAML) == 0 {
		return Session{}, ErrNoContext
	}
	owner, name := splitRepo(ctx.Repo)
	dir := filepath.Join(baseDir, owner, name, strconv.FormatInt(ctx.RunID, 10))
	if err := os.RemoveAll(dir); err != nil {
		return Session{}, fmt.Errorf("clearing %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Session{}, fmt.Errorf("creating %s: %w", dir, err)
	}
	s := Session{Context: ctx, Dir: dir}
	if len(log) > 0 {
		p := filepath.Join(dir, logFileName)
		if err := os.WriteFile(p, log, 0o644); err != nil {
			return Session{}, fmt.Errorf("writing %s: %w", p, err)
		}
		s.LogFile = p
	}
	if len(workflowYAML) > 0 {
		p := filepath.Join(dir, workflowFileName)
		if err := os.WriteFile(p, workflowYAML, 0o644); err != nil {
			return Session{}, fmt.Errorf("writing %s: %w", p, err)
		}
		s.WorkflowFile = p
	}
	return s, nil
}

// splitRepo turns "owner/name" into its two path segments. A malformed value
// still yields a usable single-segment directory rather than an error: the
// context directory is a cache location, not a contract.
func splitRepo(repo string) (owner, name string) {
	owner, name, found := strings.Cut(repo, "/")
	if !found {
		return owner, "run"
	}
	return owner, name
}
