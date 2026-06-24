package gh

import (
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2"
)

// Runner abstracts invoking the gh CLI, for testability.
type Runner interface {
	Exec(args ...string) ([]byte, error)
}

type ghRunner struct{}

// NewGHRunner returns a Runner backed by the official go-gh library.
func NewGHRunner() Runner { return ghRunner{} }

func (ghRunner) Exec(args ...string) ([]byte, error) {
	stdout, stderr, err := gh.Exec(args...)
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
