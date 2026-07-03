package explain

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// cliTimeout bounds one claude CLI invocation (spawning + generation).
const cliTimeout = 3 * time.Minute

// cmdRunner abstracts process execution for tests (same idea as gh.Runner).
type cmdRunner interface {
	run(ctx context.Context, name string, stdin string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) run(ctx context.Context, name string, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// ClaudeCLIExplainer shells out to `claude -p` (non-interactive print mode).
// Zero marginal cost through a Claude subscription, slower than the API.
type ClaudeCLIExplainer struct {
	cmd    string
	runner cmdRunner
}

// NewClaudeCLIExplainer builds a CLI explainer invoking cmd (usually "claude").
func NewClaudeCLIExplainer(cmd string) *ClaudeCLIExplainer {
	return &ClaudeCLIExplainer{cmd: cmd, runner: execRunner{}}
}

// Name is the UI source badge label.
func (c *ClaudeCLIExplainer) Name() string { return "claude (cli)" }

func (c *ClaudeCLIExplainer) Explain(ctx context.Context, req ExplainRequest) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()
	// The prompt is piped on stdin: a 64 KiB log as an argument would risk
	// exceeding ARG_MAX.
	prompt := SystemPrompt(req.Language) + "\n\n" + UserPrompt(req)
	out, err := c.runner.run(ctx, c.cmd, prompt, "-p")
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("%s: empty response", c.cmd)
	}
	return text, nil
}
