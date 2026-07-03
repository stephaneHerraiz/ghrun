package explain

import (
	"context"
	"strings"
	"time"
)

// Options tunes the Service.
type Options struct {
	Threshold   float32 // cosine similarity for a local hit (spec default 0.86)
	MaxLogBytes int     // raw-log truncation (from the end) before asking Claude
	Language    string  // language of generated explanations
}

// LocalResult is the outcome of the RAG phase.
type LocalResult struct {
	Found       bool    // exact or >= threshold: Explanation is usable as-is
	Exact       bool    // signature fast-path hit
	Explanation string  // hit text; on a miss, the best sub-threshold match ("best guess", may be empty)
	Similarity  float32 // 1.0 on an exact hit
	RAGDisabled bool    // embedder or store unavailable: nothing was searched
}

// ClaudeResult is the outcome of the generation phase.
type ClaudeResult struct {
	Explanation string
	Source      string // badge label: model name or "claude (cli)"
	Stored      bool   // the explanation was memorized in the knowledge base
}

// Service orchestrates normalize → retrieve → generate → memorize.
type Service struct {
	embedder Embedder
	chain    *Chain
	store    Store
	opts     Options
}

func NewService(embedder Embedder, chain *Chain, store Store, opts Options) *Service {
	return &Service{embedder: embedder, chain: chain, store: store, opts: opts}
}

// ResolveLocal runs the RAG phase on a raw failed log. It only fails softly:
// an unreachable Ollama or a broken store yields a miss with RAGDisabled set,
// never an error that would block the Claude phase.
func (s *Service) ResolveLocal(ctx context.Context, logText string) (LocalResult, error) {
	if s.store == nil {
		return LocalResult{RAGDisabled: true}, nil
	}
	normalized, sig := Prepare(logText)
	if entry, ok := s.store.GetBySignature(sig); ok {
		_ = s.store.Touch(sig) // best-effort
		return LocalResult{Found: true, Exact: true, Explanation: entry.Explanation, Similarity: 1}, nil
	}
	embedding, err := s.embedder.Embed(ctx, normalized)
	if err != nil {
		return LocalResult{RAGDisabled: true}, nil
	}
	matches, err := s.store.Query(embedding, 1)
	if err != nil {
		return LocalResult{RAGDisabled: true}, nil
	}
	if len(matches) == 0 {
		return LocalResult{}, nil
	}
	best := matches[0]
	if best.Similarity >= s.opts.Threshold {
		_ = s.store.Touch(best.Signature) // best-effort
		return LocalResult{Found: true, Explanation: best.Explanation, Similarity: best.Similarity}, nil
	}
	return LocalResult{Explanation: best.Explanation, Similarity: best.Similarity}, nil
}

// AskClaude generates a fresh explanation through the chain and memorizes it
// (best-effort). req.Log must be the full raw failed log; truncation to
// MaxLogBytes happens here, keeping the end of the log.
func (s *Service) AskClaude(ctx context.Context, req ExplainRequest) (ClaudeResult, error) {
	req.Language = s.opts.Language
	fullLog := req.Log
	req.Log = truncateTail(fullLog, s.opts.MaxLogBytes)
	res, err := s.chain.Explain(ctx, req)
	if err != nil {
		return ClaudeResult{}, err
	}
	stored := s.memorize(ctx, fullLog, req, res)
	return ClaudeResult{Explanation: res.Explanation, Source: res.Source, Stored: stored}, nil
}

// memorize embeds the normalized error region and upserts the entry. Every
// failure is swallowed: memorization must never break the explanation. The
// signature is computed on the full log, matching ResolveLocal.
func (s *Service) memorize(ctx context.Context, fullLog string, req ExplainRequest, res ChainResult) bool {
	if s.store == nil {
		return false
	}
	normalized, sig := Prepare(fullLog)
	embedding, err := s.embedder.Embed(ctx, normalized)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	err = s.store.Upsert(Entry{
		Signature:   sig,
		Normalized:  normalized,
		Embedding:   embedding,
		Explanation: res.Explanation,
		Repo:        req.Repo,
		Workflow:    req.Workflow,
		FailedSteps: strings.Join(req.FailedSteps, "; "),
		Model:       res.Source,
		CreatedAt:   now,
		LastUsedAt:  now,
		UseCount:    1,
		Language:    req.Language,
	})
	return err == nil
}

// truncateTail keeps the last max bytes of s (errors live at the end of a
// log), then drops everything up to and including the first newline so the
// result starts on a line boundary (when the cut already lands on a boundary
// this sacrifices one complete line, which is fine for a truncation heuristic).
func truncateTail(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := s[len(s)-max:]
	if i := strings.IndexByte(cut, '\n'); i >= 0 && i+1 < len(cut) {
		cut = cut[i+1:]
	}
	return cut
}
