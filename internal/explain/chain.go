package explain

import (
	"context"
	"errors"
	"fmt"
)

// ChainResult carries an explanation and which explainer produced it.
type ChainResult struct {
	Explanation string
	Source      string // badge label of the winning explainer
}

// Chain tries each Explainer in order (API first, CLI as fallback) and
// returns the first success. Errors accumulate so a total failure names
// every attempted source.
type Chain struct {
	Explainers []Explainer
}

func (c *Chain) Explain(ctx context.Context, req ExplainRequest) (ChainResult, error) {
	if len(c.Explainers) == 0 {
		return ChainResult{}, errors.New("no explainer available (set ANTHROPIC_API_KEY or install the claude CLI)")
	}
	var errs []error
	for _, e := range c.Explainers {
		out, err := e.Explain(ctx, req)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		return ChainResult{Explanation: out, Source: e.Name()}, nil
	}
	return ChainResult{}, errors.Join(errs...)
}
