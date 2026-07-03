package explain

import (
	"context"
	"errors"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// maxExplanationTokens bounds the generated explanation (spec: 2048).
const maxExplanationTokens = 2048

// AnthropicExplainer asks the Anthropic API directly (fast path when a key
// is configured).
type AnthropicExplainer struct {
	client anthropic.Client
	model  string
}

// NewAnthropicExplainer builds an API explainer. Extra request options are
// for tests (e.g. option.WithBaseURL against an httptest server).
func NewAnthropicExplainer(apiKey, model string, opts ...option.RequestOption) *AnthropicExplainer {
	all := append([]option.RequestOption{option.WithAPIKey(apiKey)}, opts...)
	return &AnthropicExplainer{client: anthropic.NewClient(all...), model: model}
}

// Name is the UI source badge label.
func (a *AnthropicExplainer) Name() string { return a.model }

func (a *AnthropicExplainer) Explain(ctx context.Context, req ExplainRequest) (string, error) {
	msg, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxExplanationTokens,
		System:    []anthropic.TextBlockParam{{Text: SystemPrompt(req.Language)}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(UserPrompt(req))),
		},
	})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	if strings.TrimSpace(b.String()) == "" {
		return "", errors.New("anthropic: empty response")
	}
	return b.String(), nil
}
