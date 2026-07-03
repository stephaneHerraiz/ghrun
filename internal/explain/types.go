package explain

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExplainRequest is what an Explainer needs to explain a failed run. Log is
// the raw failed log; the Service truncates it from the end before handing
// the request to an explainer.
type ExplainRequest struct {
	Log         string
	Repo        string // "owner/name"
	Workflow    string
	FailedSteps []string // "job / step"
	Language    string   // answer language, e.g. "English"; set by the Service
}

// Embedder turns text into a vector (Ollama in production).
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Explainer produces an explanation for a failed run (Anthropic API or the
// claude CLI). Name labels the UI source badge, e.g. "claude-sonnet-5".
type Explainer interface {
	Explain(ctx context.Context, req ExplainRequest) (string, error)
	Name() string
}

// Entry is one memorized explanation (one chromem document).
type Entry struct {
	Signature   string // sha256 of Normalized; chromem document ID
	Normalized  string // normalized error region (the embedded text)
	Embedding   []float32
	Explanation string
	Repo        string
	Workflow    string
	FailedSteps string // "job / step; job / step"
	Model       string // source that generated the explanation
	CreatedAt   time.Time
	LastUsedAt  time.Time
	UseCount    int
	Language    string
}

// Match is an Entry with its cosine similarity to the query.
type Match struct {
	Entry
	Similarity float32
}

// Store persists explained errors.
type Store interface {
	GetBySignature(sig string) (*Entry, bool)
	// Query returns the topK most similar entries; the Service only uses the
	// best match (topK = 1).
	Query(embedding []float32, topK int) ([]Match, error)
	Upsert(e Entry) error
	// Touch increments UseCount and refreshes LastUsedAt.
	Touch(sig string) error
}

// SystemPrompt is the fixed instruction shared by all explainers.
func SystemPrompt(language string) string {
	if language == "" {
		language = "English"
	}
	return fmt.Sprintf("Explain why this GitHub Actions run failed. Be concise: root cause first, then the failing step/command, then a suggested fix. Answer in %s.", language)
}

// UserPrompt renders the run context and raw log sent to an explainer.
func UserPrompt(req ExplainRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\n", req.Repo)
	if req.Workflow != "" {
		fmt.Fprintf(&b, "Workflow: %s\n", req.Workflow)
	}
	if len(req.FailedSteps) > 0 {
		fmt.Fprintf(&b, "Failed steps: %s\n", strings.Join(req.FailedSteps, "; "))
	}
	b.WriteString("\n--- failed log (may be truncated) ---\n")
	b.WriteString(req.Log)
	return b.String()
}
