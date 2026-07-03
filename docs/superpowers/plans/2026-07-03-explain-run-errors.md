# Explain Run Errors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the run-detail screen of a failed run, key `e` opens an Explanation screen that explains the failure from its log — instantly from a local self-enriching RAG knowledge base on a hit, or via Claude (Anthropic API, fallback claude CLI) on a miss, memorizing the fresh explanation.

**Architecture:** New `internal/explain` package (interfaces `Embedder`/`Explainer`/`Store`, mirroring the `GHClient`/`Runner` mockable philosophy): log → error-region extraction → volatile-token normalization → sha256 signature → exact fast-path or Ollama embedding + cosine query in a persistent chromem-go store (threshold 0.86) → on miss, an explainer chain (Anthropic API if a key is set, else `claude -p`) generates the explanation which is embedded and upserted. The UI adds an `explainScreen` (viewport, same pattern as `logs.go`); `rundetail` emits an `explainRunMsg` on `e` that the root `App` (which owns the service) resolves into a screen push — no constructor signature changes in `runs.go`/`launch.go`.

**Tech Stack:** Go 1.25, Bubbletea/bubbles/lipgloss (existing), `github.com/philippgille/chromem-go v0.7.0`, `github.com/anthropics/anthropic-sdk-go v1.56.0`, local Ollama HTTP API (`/api/embeddings`).

**Spec:** `docs/superpowers/specs/2026-07-03-explain-run-errors-design.md`

## Global Constraints

- Branch: `feat/explain-run-errors` (already checked out, spec committed as `4a211ef`).
- All code, identifiers, comments, commit messages, UI copy: **English**. Generated explanations default to English (`explain.language: English`).
- Similarity threshold default: **0.86**. Anthropic model default: **`claude-sonnet-5`**. `max_tokens`: **2048**. Log truncation: **65536 bytes, from the end**.
- Dependencies pinned: `chromem-go v0.7.0`, `anthropic-sdk-go v1.56.0`. No other new deps.
- No test may require a real Ollama server, a real `claude` binary, or network access.
- Every explain failure is **non-fatal** to the TUI (existing `errMsg` philosophy).
- Contextual screen keys are lowercase (project convention); run `go test ./...` (all packages) before every commit — the whole suite must stay green.
- End every commit message with:
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`

## Verified third-party API facts (do not re-derive)

- chromem-go: `chromem.NewPersistentDB(path string, compress bool) (*DB, error)`; `db.GetOrCreateCollection(name string, metadata map[string]string, embeddingFunc EmbeddingFunc) (*Collection, error)`; `coll.AddDocument(ctx, chromem.Document{ID, Content, Embedding, Metadata}) error` **overwrites an existing ID** (native upsert); `coll.GetByID(ctx, id) (Document, error)` errors when absent; `coll.QueryEmbedding(ctx, embedding, nResults, nil, nil) ([]Result, error)` **errors if `nResults <= 0` or `nResults > coll.Count()`** — clamp; `Result.Similarity` is cosine in [-1,1]; embeddings are auto-normalized on add.
- anthropic-sdk-go: `anthropic.NewClient(opts ...option.RequestOption) Client` (value, reads `ANTHROPIC_API_KEY` env by default); `option.WithAPIKey`, `option.WithBaseURL`, `option.WithMaxRetries`; `client.Messages.New(ctx, anthropic.MessageNewParams{Model anthropic.Model, MaxTokens int64, System []anthropic.TextBlockParam, Messages []anthropic.MessageParam})`; `anthropic.NewUserMessage(anthropic.NewTextBlock(...))`; response blocks via `block.AsAny().(anthropic.TextBlock)`. Constant `anthropic.ModelClaudeSonnet5 = "claude-sonnet-5"` exists but we pass the **config string** through `anthropic.Model(cfg)`.
- Ollama embeddings: `POST {base}/api/embeddings` body `{"model":"nomic-embed-text","prompt":"..."}` → `{"embedding":[...]}`.

## File Structure

```
internal/explain/
├── normalize.go        # Task 1 — error-region extraction, normalization, signature
├── normalize_test.go
├── types.go            # Task 2 — ExplainRequest, Entry, Match, interfaces, prompts
├── ollama.go           # Task 2 — Embedder over Ollama HTTP
├── ollama_test.go
├── store.go            # Task 3 — Store over chromem-go
├── store_test.go
├── anthropic.go        # Task 4 — Explainer over the Anthropic API
├── anthropic_test.go
├── claude.go           # Task 5 — Explainer shelling out to `claude -p`
├── claude_test.go
├── chain.go            # Task 6 — ordered fallback across explainers
├── chain_test.go
├── service.go          # Task 7 — orchestrator (ResolveLocal / AskClaude)
└── service_test.go
internal/config/config.go        # Task 8 — ExplainConfig + defaults + store path
internal/ui/messages.go          # Task 9 — explain messages & cmds
internal/ui/client.go            # Task 9 — ExplainService interface
internal/ui/explain.go           # Task 9 — explanation screen
internal/ui/explain_test.go
internal/ui/rundetail.go         # Task 10 — key `e`, footer hint
internal/ui/app.go               # Task 10 — service field, push hook, msg handler
cmd/ghrun/bootstrap.go           # Task 11 — buildExplainService wiring
README.md                        # Task 11 — docs
```

---

### Task 1: Normalization — error region, volatile tokens, signature

**Files:**
- Create: `internal/explain/normalize.go`
- Test: `internal/explain/normalize_test.go`

**Interfaces:**
- Consumes: nothing (pure functions, stdlib only).
- Produces: `Prepare(log string) (normalized, signature string)` — used by the Service (Task 7). Internals `extractErrorRegion(log string) string` and `normalizeVolatile(s string) string` stay unexported but are tested directly.

- [ ] **Step 1: Write the failing tests**

Create `internal/explain/normalize_test.go`:

```go
package explain

import (
	"fmt"
	"strings"
	"testing"
)

func TestExtractErrorRegionMatchesVariants(t *testing.T) {
	cases := []struct{ name, line string }{
		{"gh error marker", "##[error]Process failed"},
		{"plain error", "Error: something broke"},
		{"npm err", "npm ERR! code ELIFECYCLE"},
		{"fail lower", "1 test fail"},
		{"failed upper", "2 tests FAILED"},
		{"failure mixed", "Failure while linking"},
		{"failing", "3 failing"},
		{"fatal", "fatal: repository not found"},
		{"panic", "panic: runtime error: index out of range"},
		{"cross mark", "✗ should compile"},
		{"exit code", "Process completed with exit code 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := "line A\nline B\n" + c.line + "\ntrailing ok"
			got := extractErrorRegion(log)
			if !strings.Contains(got, c.line) {
				t.Fatalf("region %q does not contain %q", got, c.line)
			}
		})
	}
}

func TestExtractErrorRegionSkipsCleanLines(t *testing.T) {
	log := "downloading dependencies\ncompiling module alpha\nError: boom"
	got := extractErrorRegion(log)
	if !strings.Contains(got, "Error: boom") {
		t.Fatalf("region %q misses the error line", got)
	}
	// Clean lines far from any error line must not survive.
	long := strings.Repeat("clean build output\n", 20) + "unrelated middle\n" + strings.Repeat("more clean output\n", 20) + "Error: boom"
	got = extractErrorRegion(long)
	if strings.Contains(got, "unrelated middle") {
		t.Fatalf("region %q kept a line outside the context window", got)
	}
}

func TestExtractErrorRegionKeepsContextBefore(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "setup line %d\n", i)
	}
	b.WriteString("Error: boom")
	got := extractErrorRegion(b.String())
	if !strings.Contains(got, "setup line 6") {
		t.Errorf("region %q misses context line 6 (want 5 lines before)", got)
	}
	if strings.Contains(got, "setup line 5\n") {
		t.Errorf("region %q kept more than 5 context lines", got)
	}
}

func TestExtractErrorRegionFallsBackToTail(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, "ok step %d\n", i)
	}
	got := extractErrorRegion(b.String())
	if !strings.Contains(got, "ok step 100") {
		t.Errorf("fallback region %q misses the log tail", got)
	}
	if strings.Contains(got, "ok step 1\n") {
		t.Errorf("fallback region %q kept the whole log", got)
	}
}

func TestNormalizeVolatileClasses(t *testing.T) {
	cases := []struct{ name, in, wantGone, wantPlaceholder string }{
		{"timestamp", "at 2026-07-03T10:15:42.123Z step", "2026-07-03T10:15:42.123Z", "<TS>"},
		{"uuid", "request 3f2b8a10-9c4d-4e2a-8b1a-77aa00bb11cc failed", "3f2b8a10-9c4d-4e2a-8b1a-77aa00bb11cc", "<UUID>"},
		{"sha", "commit a1b2c3d4e5f failed", "a1b2c3d4e5f", "<SHA>"},
		{"ip port", "dial 192.168.1.10:8080 refused", "192.168.1.10:8080", "<IP>"},
		{"abs path", "open /home/runner/work/app/main.go failed", "/home/runner/work/app", "<PATH>"},
		{"tmp path", "wrote /tmp/build-artifacts/out.bin then failed", "/tmp/build-artifacts", "<PATH>"},
		{"duration", "test timed out after 300s of failure", "300s", "<DUR>"},
		{"line number", "main.go:127:3: undefined: foo error", ":127:3", "<LN>"},
		// 5 digits: below the SHA pattern's 7-char floor, so <NUM> catches it.
		// Longer all-digit runs are hex-valid and become <SHA> — mislabeled but
		// stable, which is all the signature needs.
		{"long id", "run 12345 failed", "12345", "<NUM>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeVolatile(c.in)
			if strings.Contains(got, c.wantGone) {
				t.Errorf("normalizeVolatile(%q) = %q — still contains %q", c.in, got, c.wantGone)
			}
			if !strings.Contains(got, c.wantPlaceholder) {
				t.Errorf("normalizeVolatile(%q) = %q — missing %q", c.in, got, c.wantPlaceholder)
			}
		})
	}
}

func TestPrepareSignatureStable(t *testing.T) {
	logA := "build\t2026-07-03T10:00:01Z compiling /home/runner/work/a.go\nbuild\t2026-07-03T10:00:02Z Error: undefined symbol (run 111222333)"
	logB := "build\t2026-07-04T22:59:59Z compiling /home/other/dir/a.go\nbuild\t2026-07-04T23:00:00Z Error: undefined symbol (run 999888777)"
	_, sigA := Prepare(logA)
	_, sigB := Prepare(logB)
	if sigA != sigB {
		t.Errorf("signatures differ for volatile-only changes:\n%s\n%s", sigA, sigB)
	}
	_, sigC := Prepare("build\tError: a completely different failure")
	if sigC == sigA {
		t.Errorf("different errors share signature %s", sigC)
	}
	if len(sigA) != 64 {
		t.Errorf("signature %q is not a sha256 hex string", sigA)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/explain/ -v`
Expected: FAIL — `undefined: extractErrorRegion`, `undefined: normalizeVolatile`, `undefined: Prepare` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/explain/normalize.go`:

```go
// Package explain turns a failed GitHub Actions log into an explanation,
// backed by a self-enriching local RAG knowledge base and Claude.
package explain

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// contextBefore is how many lines of context are kept before each error line.
const contextBefore = 5

// maxRegionLines caps the extracted region so the embedded text stays focused.
const maxRegionLines = 400

// fallbackTailLines is used when no error marker matches at all.
const fallbackTailLines = 30

// errLineRe matches lines that look like errors, case-insensitively. The net
// is deliberately wide (fail/failed/failure/failing, fatal, panic, npm ERR!,
// exit code…): over-extracting beats missing the root cause. "exit code" also
// covers "Process completed with exit code N".
var errLineRe = regexp.MustCompile(`(?i)##\[error\]|\berror\b|\berr!|\bfail(ed|ure|ing)?\b|\bfatal\b|\bpanic\b|✗|exit code`)

// volatileRes maps volatile-token patterns to placeholders. Order matters:
// more specific patterns run first (timestamp before date/time, UUID before
// SHA, IP:port before bare line numbers).
var volatileRes = []struct {
	re  *regexp.Regexp
	rep string
}{
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`), "<TS>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}`), "<DATE>"},
	{regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}(\.\d+)?\b`), "<TIME>"},
	{regexp.MustCompile(`\b\d+(\.\d+)?(ms|s|m|h)\b`), "<DUR>"},
	{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), "<UUID>"},
	{regexp.MustCompile(`\b[0-9a-f]{7,40}\b`), "<SHA>"},
	{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?\b`), "<IP>"},
	{regexp.MustCompile(`/(?:home|tmp|__w|__t|var|usr|opt|Users|runner|github)[\w@./-]*`), "<PATH>"},
	{regexp.MustCompile(`:\d+(:\d+)?\b`), "<LN>"},
	{regexp.MustCompile(`\b\d{5,}\b`), "<NUM>"},
}

// extractErrorRegion keeps every line matching errLineRe plus contextBefore
// lines before each. With no match at all it falls back to the log tail, so
// there is always something to embed.
func extractErrorRegion(log string) string {
	lines := strings.Split(log, "\n")
	keep := make([]bool, len(lines))
	found := false
	for i, l := range lines {
		if errLineRe.MatchString(l) {
			found = true
			start := i - contextBefore
			if start < 0 {
				start = 0
			}
			for j := start; j <= i; j++ {
				keep[j] = true
			}
		}
	}
	if !found {
		start := len(lines) - fallbackTailLines
		if start < 0 {
			start = 0
		}
		return strings.Join(lines[start:], "\n")
	}
	var out []string
	for i, k := range keep {
		if k {
			out = append(out, lines[i])
		}
	}
	if len(out) > maxRegionLines {
		out = out[len(out)-maxRegionLines:]
	}
	return strings.Join(out, "\n")
}

// normalizeVolatile replaces volatile tokens (timestamps, paths, SHAs, IDs…)
// with placeholders so identical errors from different runs converge.
func normalizeVolatile(s string) string {
	for _, v := range volatileRes {
		s = v.re.ReplaceAllString(s, v.rep)
	}
	return s
}

// Prepare turns a raw failed log into the normalized text that is embedded
// and compared, plus its sha256 signature (exact-match fast path and chromem
// document ID).
func Prepare(log string) (normalized, signature string) {
	normalized = normalizeVolatile(extractErrorRegion(log))
	sum := sha256.Sum256([]byte(normalized))
	return normalized, hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/explain/ -v`
Expected: PASS (all `TestExtract*`, `TestNormalize*`, `TestPrepare*`).

- [ ] **Step 5: Run the full suite and commit**

```bash
go test ./...
git add internal/explain/normalize.go internal/explain/normalize_test.go
git commit -m "feat(explain): error-region extraction, normalization and signatures

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Types, prompts and Ollama embedder

**Files:**
- Create: `internal/explain/types.go`
- Create: `internal/explain/ollama.go`
- Test: `internal/explain/ollama_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces (used by every later task):
  - `type ExplainRequest struct { Log, Repo, Workflow string; FailedSteps []string; Language string }`
  - `type Embedder interface { Embed(ctx context.Context, text string) ([]float32, error) }`
  - `type Explainer interface { Explain(ctx context.Context, req ExplainRequest) (string, error); Name() string }`
  - `type Entry struct { Signature, Normalized string; Embedding []float32; Explanation, Repo, Workflow, FailedSteps, Model string; CreatedAt, LastUsedAt time.Time; UseCount int; Language string }`
  - `type Match struct { Entry; Similarity float32 }`
  - `type Store interface { GetBySignature(sig string) (*Entry, bool); Query(embedding []float32, topK int) ([]Match, error); Upsert(e Entry) error; Touch(sig string) error }`
  - `SystemPrompt(language string) string`, `UserPrompt(req ExplainRequest) string`
  - `NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder` (implements `Embedder`)

- [ ] **Step 1: Write the failing test**

Create `internal/explain/ollama_test.go`:

```go
package explain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaEmbedderPostsAndDecodes(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"embedding":[0.1,0.2,0.3]}`))
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL+"/", "nomic-embed-text") // trailing slash must be tolerated
	vec, err := e.Embed(context.Background(), "Error: boom")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 3 || vec[1] != 0.2 {
		t.Errorf("vec = %v", vec)
	}
	if gotPath != "/api/embeddings" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["model"] != "nomic-embed-text" || gotBody["prompt"] != "Error: boom" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestOllamaEmbedderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()
	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	if _, err := e.Embed(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("want 404 error, got %v", err)
	}
}

func TestOllamaEmbedderEmptyEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"embedding":[]}`))
	}))
	defer srv.Close()
	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	if _, err := e.Embed(context.Background(), "x"); err == nil {
		t.Error("want error on empty embedding")
	}
}

func TestPrompts(t *testing.T) {
	sys := SystemPrompt("English")
	if !strings.Contains(sys, "GitHub Actions") || !strings.Contains(sys, "Answer in English") {
		t.Errorf("system prompt = %q", sys)
	}
	if got := SystemPrompt(""); !strings.Contains(got, "Answer in English") {
		t.Errorf("empty language must default to English, got %q", got)
	}
	up := UserPrompt(ExplainRequest{
		Log: "raw log", Repo: "o/r", Workflow: "CI",
		FailedSteps: []string{"build / test"},
	})
	for _, want := range []string{"o/r", "CI", "build / test", "raw log"} {
		if !strings.Contains(up, want) {
			t.Errorf("user prompt %q misses %q", up, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/explain/ -v -run 'TestOllama|TestPrompts'`
Expected: FAIL — `undefined: NewOllamaEmbedder`, `undefined: SystemPrompt`, `undefined: ExplainRequest` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/explain/types.go`:

```go
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
```

Create `internal/explain/ollama.go`:

```go
package explain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaEmbedder implements Embedder against a local Ollama server.
type OllamaEmbedder struct {
	baseURL string
	model   string
	httpc   *http.Client
}

// NewOllamaEmbedder builds an embedder for POST {baseURL}/api/embeddings.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		httpc:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{"model": o.model, "prompt": text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama: decoding response: %w", err)
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding (is model %q pulled?)", o.model)
	}
	return out.Embedding, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/explain/ -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite and commit**

```bash
go test ./...
git add internal/explain/types.go internal/explain/ollama.go internal/explain/ollama_test.go
git commit -m "feat(explain): interfaces, prompts and Ollama embedder

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: chromem-go persistent store

**Files:**
- Create: `internal/explain/store.go`
- Test: `internal/explain/store_test.go`
- Modify: `go.mod` / `go.sum` (via `go get`)

**Interfaces:**
- Consumes: `Entry`, `Match`, `Store` from Task 2.
- Produces: `NewChromemStore(path string) (*ChromemStore, error)` — `*ChromemStore` implements `Store`. A corrupted/unreadable store directory is wiped and recreated empty (non-fatal per spec).

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/philippgille/chromem-go@v0.7.0`
Expected: go.mod gains `github.com/philippgille/chromem-go v0.7.0`.

- [ ] **Step 2: Write the failing tests**

Create `internal/explain/store_test.go`:

```go
package explain

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testEntry(sig string) Entry {
	return Entry{
		Signature:   sig,
		Normalized:  "build <TS> Error: undefined symbol",
		Embedding:   []float32{1, 0, 0},
		Explanation: "The linker failed because the symbol is undefined.",
		Repo:        "o/r",
		Workflow:    "CI",
		FailedSteps: "build / link",
		Model:       "claude-sonnet-5",
		CreatedAt:   time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC),
		LastUsedAt:  time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC),
		UseCount:    1,
		Language:    "English",
	}
}

func TestStoreUpsertAndGetBySignature(t *testing.T) {
	s, err := NewChromemStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	if _, ok := s.GetBySignature("missing"); ok {
		t.Fatal("empty store returned an entry")
	}
	want := testEntry("sig-1")
	if err := s.Upsert(want); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, ok := s.GetBySignature("sig-1")
	if !ok {
		t.Fatal("entry not found after upsert")
	}
	if got.Explanation != want.Explanation || got.Repo != "o/r" || got.UseCount != 1 ||
		got.Model != "claude-sonnet-5" || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("got %+v", got)
	}
}

func TestStoreQueryEmptyAndSimilarity(t *testing.T) {
	s, err := NewChromemStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	// Empty store: no error, no matches (chromem errors when nResults > count).
	if m, err := s.Query([]float32{1, 0, 0}, 1); err != nil || len(m) != 0 {
		t.Fatalf("empty query: m=%v err=%v", m, err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	m, err := s.Query([]float32{1, 0, 0}, 5) // topK beyond count must be clamped
	if err != nil || len(m) != 1 {
		t.Fatalf("query: m=%v err=%v", m, err)
	}
	if m[0].Similarity < 0.99 {
		t.Errorf("identical vector similarity = %f", m[0].Similarity)
	}
	if m[0].Explanation == "" || m[0].Signature != "sig-1" {
		t.Errorf("match entry not hydrated: %+v", m[0].Entry)
	}
}

func TestStoreTouchIncrements(t *testing.T) {
	s, err := NewChromemStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.Touch("sig-1"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, _ := s.GetBySignature("sig-1")
	if got.UseCount != 2 {
		t.Errorf("UseCount = %d, want 2", got.UseCount)
	}
	if !got.LastUsedAt.After(testEntry("sig-1").LastUsedAt) {
		t.Errorf("LastUsedAt not refreshed: %v", got.LastUsedAt)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	s, err := NewChromemStore(dir)
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	s2, err := NewChromemStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok := s2.GetBySignature("sig-1"); !ok {
		t.Error("entry lost after reopen")
	}
}

func TestStoreCorruptedRecreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db")
	// A regular file where the DB directory should be makes chromem fail.
	if err := os.WriteFile(path, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewChromemStore(path)
	if err != nil {
		t.Fatalf("corrupted store not recreated: %v", err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Errorf("Upsert after recreation: %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/explain/ -v -run TestStore`
Expected: FAIL — `undefined: NewChromemStore` (compile error).

- [ ] **Step 4: Write the implementation**

Create `internal/explain/store.go`:

```go
package explain

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	chromem "github.com/philippgille/chromem-go"
)

const collectionName = "explanations"

// ChromemStore persists entries in an embedded chromem-go database.
type ChromemStore struct {
	coll *chromem.Collection
}

// NewChromemStore opens (or creates) the persistent store at path. A
// corrupted or unreadable store is wiped and recreated empty: losing the
// cache must never break the feature.
func NewChromemStore(path string) (*ChromemStore, error) {
	coll, err := openCollection(path)
	if err != nil {
		if rmErr := os.RemoveAll(path); rmErr != nil {
			return nil, errors.Join(err, rmErr)
		}
		coll, err = openCollection(path)
		if err != nil {
			return nil, err
		}
	}
	return &ChromemStore{coll: coll}, nil
}

func openCollection(path string) (*chromem.Collection, error) {
	db, err := chromem.NewPersistentDB(path, false)
	if err != nil {
		return nil, err
	}
	return db.GetOrCreateCollection(collectionName, nil, noEmbedding)
}

// noEmbedding guards the collection: embeddings are always provided
// explicitly, chromem must never compute one itself.
func noEmbedding(context.Context, string) ([]float32, error) {
	return nil, errors.New("explain: embedding function should never be called")
}

func (s *ChromemStore) GetBySignature(sig string) (*Entry, bool) {
	doc, err := s.coll.GetByID(context.Background(), sig)
	if err != nil {
		return nil, false
	}
	e := docToEntry(doc.ID, doc.Content, doc.Embedding, doc.Metadata)
	return &e, true
}

func (s *ChromemStore) Query(embedding []float32, topK int) ([]Match, error) {
	n := s.coll.Count()
	if n == 0 || topK <= 0 {
		return nil, nil
	}
	if topK > n {
		topK = n // chromem errors when nResults > document count
	}
	results, err := s.coll.QueryEmbedding(context.Background(), embedding, topK, nil, nil)
	if err != nil {
		return nil, err
	}
	matches := make([]Match, 0, len(results))
	for _, r := range results {
		matches = append(matches, Match{
			Entry:      docToEntry(r.ID, r.Content, r.Embedding, r.Metadata),
			Similarity: r.Similarity,
		})
	}
	return matches, nil
}

func (s *ChromemStore) Upsert(e Entry) error {
	// AddDocument overwrites an existing ID: native upsert.
	return s.coll.AddDocument(context.Background(), chromem.Document{
		ID:        e.Signature,
		Content:   e.Normalized,
		Embedding: e.Embedding,
		Metadata:  entryMetadata(e),
	})
}

func (s *ChromemStore) Touch(sig string) error {
	doc, err := s.coll.GetByID(context.Background(), sig)
	if err != nil {
		return err
	}
	doc.Metadata["useCount"] = strconv.Itoa(atoiOr(doc.Metadata["useCount"], 0) + 1)
	doc.Metadata["lastUsedAt"] = time.Now().UTC().Format(time.RFC3339)
	return s.coll.AddDocument(context.Background(), doc)
}

func entryMetadata(e Entry) map[string]string {
	return map[string]string{
		"explanation": e.Explanation,
		"repo":        e.Repo,
		"workflow":    e.Workflow,
		"failedSteps": e.FailedSteps,
		"model":       e.Model,
		"createdAt":   e.CreatedAt.UTC().Format(time.RFC3339),
		"lastUsedAt":  e.LastUsedAt.UTC().Format(time.RFC3339),
		"useCount":    strconv.Itoa(e.UseCount),
		"language":    e.Language,
	}
}

func docToEntry(id, content string, embedding []float32, meta map[string]string) Entry {
	createdAt, _ := time.Parse(time.RFC3339, meta["createdAt"])
	lastUsedAt, _ := time.Parse(time.RFC3339, meta["lastUsedAt"])
	return Entry{
		Signature:   id,
		Normalized:  content,
		Embedding:   embedding,
		Explanation: meta["explanation"],
		Repo:        meta["repo"],
		Workflow:    meta["workflow"],
		FailedSteps: meta["failedSteps"],
		Model:       meta["model"],
		CreatedAt:   createdAt,
		LastUsedAt:  lastUsedAt,
		UseCount:    atoiOr(meta["useCount"], 0),
		Language:    meta["language"],
	}
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/explain/ -v -run TestStore`
Expected: PASS (5 tests).

- [ ] **Step 6: Run the full suite and commit**

```bash
go test ./... && go mod tidy
git add go.mod go.sum internal/explain/store.go internal/explain/store_test.go
git commit -m "feat(explain): chromem-go persistent store

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Anthropic API explainer

**Files:**
- Create: `internal/explain/anthropic.go`
- Test: `internal/explain/anthropic_test.go`
- Modify: `go.mod` / `go.sum` (via `go get`)

**Interfaces:**
- Consumes: `ExplainRequest`, `Explainer`, `SystemPrompt`, `UserPrompt` from Task 2.
- Produces: `NewAnthropicExplainer(apiKey, model string, opts ...option.RequestOption) *AnthropicExplainer` — implements `Explainer`; `Name()` returns the model string (e.g. `"claude-sonnet-5"`). Extra options exist for tests (`option.WithBaseURL`).

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/anthropics/anthropic-sdk-go@v1.56.0`
Expected: go.mod gains `github.com/anthropics/anthropic-sdk-go v1.56.0`.

- [ ] **Step 2: Write the failing tests**

Create `internal/explain/anthropic_test.go`:

```go
package explain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
)

func anthropicOK(t *testing.T, capture *map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			_ = json.NewDecoder(r.Body).Decode(capture)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model": "claude-sonnet-5",
			"content": [{"type": "text", "text": "Root cause: missing dependency."}],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}
}

func TestAnthropicExplainerSendsPromptAndParsesText(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(anthropicOK(t, &got))
	defer srv.Close()

	e := NewAnthropicExplainer("test-key", "claude-sonnet-5",
		option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	out, err := e.Explain(context.Background(), ExplainRequest{
		Log: "Error: cannot find module", Repo: "o/r", Workflow: "CI", Language: "English",
	})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if out != "Root cause: missing dependency." {
		t.Errorf("out = %q", out)
	}
	if got["model"] != "claude-sonnet-5" {
		t.Errorf("model = %v", got["model"])
	}
	if got["max_tokens"] != float64(2048) {
		t.Errorf("max_tokens = %v", got["max_tokens"])
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "GitHub Actions run failed") {
		t.Errorf("system prompt missing from request: %s", raw)
	}
	if !strings.Contains(string(raw), "cannot find module") {
		t.Errorf("log missing from request: %s", raw)
	}
	if e.Name() != "claude-sonnet-5" {
		t.Errorf("Name() = %q", e.Name())
	}
}

func TestAnthropicExplainerServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"type":"error","error":{"type":"api_error","message":"boom"}}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	e := NewAnthropicExplainer("test-key", "claude-sonnet-5",
		option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	if _, err := e.Explain(context.Background(), ExplainRequest{Log: "x"}); err == nil {
		t.Error("want error on 500")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/explain/ -v -run TestAnthropic`
Expected: FAIL — `undefined: NewAnthropicExplainer` (compile error).

- [ ] **Step 4: Write the implementation**

Create `internal/explain/anthropic.go`:

```go
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/explain/ -v -run TestAnthropic`
Expected: PASS (2 tests).

- [ ] **Step 6: Run the full suite and commit**

```bash
go test ./... && go mod tidy
git add go.mod go.sum internal/explain/anthropic.go internal/explain/anthropic_test.go
git commit -m "feat(explain): Anthropic API explainer

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: claude CLI explainer

**Files:**
- Create: `internal/explain/claude.go`
- Test: `internal/explain/claude_test.go`

**Interfaces:**
- Consumes: `ExplainRequest`, `Explainer`, `SystemPrompt`, `UserPrompt` from Task 2.
- Produces: `NewClaudeCLIExplainer(cmd string) *ClaudeCLIExplainer` — implements `Explainer`; `Name()` returns `"claude (cli)"`. Internal `cmdRunner` interface (`run(ctx, name, stdin string, args ...string) ([]byte, error)`) mirrors `gh.Runner` for testability.

- [ ] **Step 1: Write the failing tests**

Create `internal/explain/claude_test.go`:

```go
package explain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	out      []byte
	err      error
	gotName  string
	gotStdin string
	gotArgs  []string
}

func (f *fakeRunner) run(_ context.Context, name string, stdin string, args ...string) ([]byte, error) {
	f.gotName, f.gotStdin, f.gotArgs = name, stdin, args
	return f.out, f.err
}

func TestClaudeCLIExplainerPipesPromptOnStdin(t *testing.T) {
	fr := &fakeRunner{out: []byte("The build failed because...\n")}
	e := NewClaudeCLIExplainer("claude")
	e.runner = fr
	out, err := e.Explain(context.Background(), ExplainRequest{
		Log: "Error: oom", Repo: "o/r", Language: "English",
	})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if out != "The build failed because..." {
		t.Errorf("out = %q (want trimmed)", out)
	}
	if fr.gotName != "claude" || len(fr.gotArgs) != 1 || fr.gotArgs[0] != "-p" {
		t.Errorf("invoked %q %v", fr.gotName, fr.gotArgs)
	}
	// The prompt goes through stdin (a 64 KiB log would blow past ARG_MAX).
	if !strings.Contains(fr.gotStdin, "GitHub Actions run failed") ||
		!strings.Contains(fr.gotStdin, "Error: oom") {
		t.Errorf("stdin = %q", fr.gotStdin)
	}
	if e.Name() != "claude (cli)" {
		t.Errorf("Name() = %q", e.Name())
	}
}

func TestClaudeCLIExplainerErrors(t *testing.T) {
	e := NewClaudeCLIExplainer("claude")
	e.runner = &fakeRunner{err: errors.New("exec: not found")}
	if _, err := e.Explain(context.Background(), ExplainRequest{Log: "x"}); err == nil {
		t.Error("want error when the CLI fails")
	}
	e.runner = &fakeRunner{out: []byte("   \n")}
	if _, err := e.Explain(context.Background(), ExplainRequest{Log: "x"}); err == nil {
		t.Error("want error on empty output")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/explain/ -v -run TestClaudeCLI`
Expected: FAIL — `undefined: NewClaudeCLIExplainer` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/explain/claude.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/explain/ -v -run TestClaudeCLI`
Expected: PASS (2 tests).

- [ ] **Step 5: Run the full suite and commit**

```bash
go test ./...
git add internal/explain/claude.go internal/explain/claude_test.go
git commit -m "feat(explain): claude CLI explainer

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 6: Explainer chain (API → CLI fallback)

**Files:**
- Create: `internal/explain/chain.go`
- Test: `internal/explain/chain_test.go`

**Interfaces:**
- Consumes: `Explainer`, `ExplainRequest` from Task 2.
- Produces: `type Chain struct { Explainers []Explainer }` with `Explain(ctx, req) (ChainResult, error)`; `type ChainResult struct { Explanation, Source string }`. The Service (Task 7) holds a `*Chain`. `fakeExplainer` (defined in the test file) is reused by `service_test.go`.

- [ ] **Step 1: Write the failing tests**

Create `internal/explain/chain_test.go`:

```go
package explain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeExplainer is shared with service_test.go (same package).
type fakeExplainer struct {
	name   string
	out    string
	err    error
	calls  int
	gotReq ExplainRequest
}

func (f *fakeExplainer) Explain(_ context.Context, req ExplainRequest) (string, error) {
	f.calls++
	f.gotReq = req
	return f.out, f.err
}
func (f *fakeExplainer) Name() string { return f.name }

func TestChainFirstSuccessWins(t *testing.T) {
	api := &fakeExplainer{name: "claude-sonnet-5", out: "api explanation"}
	cli := &fakeExplainer{name: "claude (cli)", out: "cli explanation"}
	c := &Chain{Explainers: []Explainer{api, cli}}
	res, err := c.Explain(context.Background(), ExplainRequest{Log: "x"})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if res.Explanation != "api explanation" || res.Source != "claude-sonnet-5" {
		t.Errorf("res = %+v", res)
	}
	if cli.calls != 0 {
		t.Error("CLI must not be called when the API succeeds")
	}
}

func TestChainFallsBackOnError(t *testing.T) {
	api := &fakeExplainer{name: "claude-sonnet-5", err: errors.New("401 unauthorized")}
	cli := &fakeExplainer{name: "claude (cli)", out: "cli explanation"}
	c := &Chain{Explainers: []Explainer{api, cli}}
	res, err := c.Explain(context.Background(), ExplainRequest{Log: "x"})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if res.Source != "claude (cli)" {
		t.Errorf("source = %q", res.Source)
	}
}

func TestChainAllFail(t *testing.T) {
	api := &fakeExplainer{name: "claude-sonnet-5", err: errors.New("network down")}
	cli := &fakeExplainer{name: "claude (cli)", err: errors.New("binary missing")}
	c := &Chain{Explainers: []Explainer{api, cli}}
	_, err := c.Explain(context.Background(), ExplainRequest{Log: "x"})
	if err == nil {
		t.Fatal("want error when every explainer fails")
	}
	for _, want := range []string{"claude-sonnet-5", "network down", "claude (cli)", "binary missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q misses %q", err, want)
		}
	}
}

func TestChainEmpty(t *testing.T) {
	c := &Chain{}
	_, err := c.Explain(context.Background(), ExplainRequest{Log: "x"})
	if err == nil || !strings.Contains(err.Error(), "no explainer") {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/explain/ -v -run TestChain`
Expected: FAIL — `undefined: Chain` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/explain/chain.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/explain/ -v -run TestChain`
Expected: PASS (4 tests).

- [ ] **Step 5: Run the full suite and commit**

```bash
go test ./...
git add internal/explain/chain.go internal/explain/chain_test.go
git commit -m "feat(explain): explainer chain with API-to-CLI fallback

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 7: Service — orchestrate RAG, Claude, memorization

**Files:**
- Create: `internal/explain/service.go`
- Test: `internal/explain/service_test.go`

**Interfaces:**
- Consumes: `Prepare` (Task 1); `Embedder`, `Store`, `Entry`, `Match`, `ExplainRequest` (Task 2); `Chain`, `ChainResult`, `fakeExplainer` (Task 6).
- Produces (consumed by the UI in Task 9):
  - `type Options struct { Threshold float32; MaxLogBytes int; Language string }`
  - `NewService(embedder Embedder, chain *Chain, store Store, opts Options) *Service`
  - `func (s *Service) ResolveLocal(ctx context.Context, logText string) (LocalResult, error)`
  - `func (s *Service) AskClaude(ctx context.Context, req ExplainRequest) (ClaudeResult, error)`
  - `type LocalResult struct { Found, Exact bool; Explanation string; Similarity float32; RAGDisabled bool }`
  - `type ClaudeResult struct { Explanation, Source string; Stored bool }`

- [ ] **Step 1: Write the failing tests**

Create `internal/explain/service_test.go`:

```go
package explain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeEmbedder struct {
	vec   []float32
	err   error
	calls int
}

func (f *fakeEmbedder) Embed(context.Context, string) ([]float32, error) {
	f.calls++
	return f.vec, f.err
}

type fakeStore struct {
	bySig    map[string]*Entry
	matches  []Match
	queryErr error
	upserts  []Entry
	touched  []string
}

func (f *fakeStore) GetBySignature(sig string) (*Entry, bool) {
	e, ok := f.bySig[sig]
	return e, ok
}
func (f *fakeStore) Query([]float32, int) ([]Match, error) { return f.matches, f.queryErr }
func (f *fakeStore) Upsert(e Entry) error                  { f.upserts = append(f.upserts, e); return nil }
func (f *fakeStore) Touch(sig string) error                { f.touched = append(f.touched, sig); return nil }

func newTestService(emb *fakeEmbedder, st Store, ex ...Explainer) *Service {
	return NewService(emb, &Chain{Explainers: ex}, st,
		Options{Threshold: 0.86, MaxLogBytes: 65536, Language: "English"})
}

const testLog = "build\t2026-07-03T10:00:00Z Error: undefined symbol foo"

func TestResolveLocalExactHit(t *testing.T) {
	_, sig := Prepare(testLog)
	st := &fakeStore{bySig: map[string]*Entry{sig: {Signature: sig, Explanation: "known failure"}}}
	svc := newTestService(&fakeEmbedder{}, st)
	res, err := svc.ResolveLocal(context.Background(), testLog)
	if err != nil {
		t.Fatalf("ResolveLocal: %v", err)
	}
	if !res.Found || !res.Exact || res.Explanation != "known failure" || res.Similarity != 1 {
		t.Errorf("res = %+v", res)
	}
	if len(st.touched) != 1 || st.touched[0] != sig {
		t.Errorf("touched = %v", st.touched)
	}
}

func TestResolveLocalSimilarityHit(t *testing.T) {
	st := &fakeStore{matches: []Match{{Entry: Entry{Signature: "s2", Explanation: "similar failure"}, Similarity: 0.91}}}
	svc := newTestService(&fakeEmbedder{vec: []float32{1, 0}}, st)
	res, err := svc.ResolveLocal(context.Background(), testLog)
	if err != nil {
		t.Fatalf("ResolveLocal: %v", err)
	}
	if !res.Found || res.Exact || res.Explanation != "similar failure" || res.Similarity != 0.91 {
		t.Errorf("res = %+v", res)
	}
	if len(st.touched) != 1 {
		t.Errorf("touched = %v", st.touched)
	}
}

func TestResolveLocalBelowThresholdIsBestGuess(t *testing.T) {
	st := &fakeStore{matches: []Match{{Entry: Entry{Explanation: "vaguely similar"}, Similarity: 0.55}}}
	svc := newTestService(&fakeEmbedder{vec: []float32{1, 0}}, st)
	res, _ := svc.ResolveLocal(context.Background(), testLog)
	if res.Found {
		t.Error("0.55 < 0.86 must be a miss")
	}
	if res.Explanation != "vaguely similar" || res.Similarity != 0.55 {
		t.Errorf("best guess not carried: %+v", res)
	}
	if len(st.touched) != 0 {
		t.Error("a miss must not touch the entry")
	}
}

func TestResolveLocalOllamaDown(t *testing.T) {
	svc := newTestService(&fakeEmbedder{err: errors.New("connection refused")}, &fakeStore{})
	res, err := svc.ResolveLocal(context.Background(), testLog)
	if err != nil {
		t.Fatalf("Ollama down must be a soft miss, got %v", err)
	}
	if res.Found || !res.RAGDisabled {
		t.Errorf("res = %+v", res)
	}
}

func TestResolveLocalNilStore(t *testing.T) {
	svc := NewService(&fakeEmbedder{}, &Chain{}, nil, Options{Threshold: 0.86})
	res, err := svc.ResolveLocal(context.Background(), testLog)
	if err != nil || res.Found || !res.RAGDisabled {
		t.Errorf("res = %+v err = %v", res, err)
	}
}

func TestAskClaudeGeneratesAndMemorizes(t *testing.T) {
	emb := &fakeEmbedder{vec: []float32{1, 0}}
	st := &fakeStore{}
	ex := &fakeExplainer{name: "claude-sonnet-5", out: "fresh explanation"}
	svc := newTestService(emb, st, ex)
	res, err := svc.AskClaude(context.Background(), ExplainRequest{
		Log: testLog, Repo: "o/r", Workflow: "CI", FailedSteps: []string{"build / link"},
	})
	if err != nil {
		t.Fatalf("AskClaude: %v", err)
	}
	if res.Explanation != "fresh explanation" || res.Source != "claude-sonnet-5" || !res.Stored {
		t.Errorf("res = %+v", res)
	}
	if ex.gotReq.Language != "English" {
		t.Errorf("language not injected: %+v", ex.gotReq)
	}
	if len(st.upserts) != 1 {
		t.Fatalf("upserts = %d", len(st.upserts))
	}
	up := st.upserts[0]
	_, wantSig := Prepare(testLog)
	if up.Signature != wantSig || up.Explanation != "fresh explanation" ||
		up.Model != "claude-sonnet-5" || up.FailedSteps != "build / link" ||
		up.UseCount != 1 || len(up.Embedding) == 0 {
		t.Errorf("upsert = %+v", up)
	}
}

func TestAskClaudeTruncatesLogFromEnd(t *testing.T) {
	ex := &fakeExplainer{name: "x", out: "ok"}
	svc := NewService(&fakeEmbedder{vec: []float32{1}}, &Chain{Explainers: []Explainer{ex}}, &fakeStore{},
		Options{Threshold: 0.86, MaxLogBytes: 100, Language: "English"})
	long := strings.Repeat("early line\n", 50) + "THE END MARKER"
	if _, err := svc.AskClaude(context.Background(), ExplainRequest{Log: long}); err != nil {
		t.Fatalf("AskClaude: %v", err)
	}
	if len(ex.gotReq.Log) > 100 {
		t.Errorf("log not truncated: %d bytes", len(ex.gotReq.Log))
	}
	if !strings.Contains(ex.gotReq.Log, "THE END MARKER") {
		t.Errorf("truncation must keep the end: %q", ex.gotReq.Log)
	}
}

func TestAskClaudeEmbedFailureStillReturns(t *testing.T) {
	ex := &fakeExplainer{name: "x", out: "explanation"}
	st := &fakeStore{}
	svc := newTestService(&fakeEmbedder{err: errors.New("down")}, st, ex)
	res, err := svc.AskClaude(context.Background(), ExplainRequest{Log: testLog})
	if err != nil || res.Explanation != "explanation" {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	if res.Stored || len(st.upserts) != 0 {
		t.Error("must not store without an embedding")
	}
}

func TestAskClaudeChainError(t *testing.T) {
	ex := &fakeExplainer{name: "x", err: errors.New("all down")}
	svc := newTestService(&fakeEmbedder{vec: []float32{1}}, &fakeStore{}, ex)
	if _, err := svc.AskClaude(context.Background(), ExplainRequest{Log: testLog}); err == nil {
		t.Error("chain failure must surface")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/explain/ -v -run 'TestResolveLocal|TestAskClaude'`
Expected: FAIL — `undefined: NewService`, `undefined: Options` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/explain/service.go`:

```go
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
// log), dropping the first partial line for cleanliness.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/explain/ -v`
Expected: PASS — the whole explain package.

- [ ] **Step 5: Run the full suite and commit**

```bash
go test ./...
git add internal/explain/service.go internal/explain/service_test.go
git commit -m "feat(explain): service orchestrating RAG, Claude and memorization

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 8: Config — `explain` section

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (append)

**Interfaces:**
- Consumes: existing `Config`, `Default()`, `applyDefaults`, `resolveBase`.
- Produces (used by bootstrap in Task 11): `Config.Explain ExplainConfig` with fields `Enabled *bool` (nil ⇒ enabled), `OllamaURL`, `EmbeddingModel`, `SimilarityThreshold float64`, `AnthropicAPIKey`, `Model`, `ClaudeCmd`, `StorePath`, `MaxLogBytes int`, `Language`; method `IsEnabled() bool`; function `ResolveExplainStorePath() (string, error)`.

- [ ] **Step 1: Update the round-trip test and write the failing tests**

The existing `TestSaveLoadRoundTrip` in `internal/config/config_test.go`
compares the loaded config with `reflect.DeepEqual` against its input; once
`applyDefaults` fills the new `Explain` fields, a zero-valued `Explain` input
no longer round-trips. Fix it by giving the input the explain defaults —
change the `in` literal to:

```go
	in := Config{
		DefaultOrg:             "stephaneHerraiz",
		RefreshIntervalSeconds: 6,
		RunListLimit:           50,
		ListPageSize:           25,
		Favorites:              []string{"stephaneHerraiz/ghrun"},
		Explain:                Default().Explain,
	}
```

Add `"os"` to the test file's import block (used below), then append:

```go
func TestExplainDefaults(t *testing.T) {
	c := Default().Explain
	if !c.IsEnabled() {
		t.Error("explain must default to enabled")
	}
	if c.OllamaURL != "http://localhost:11434" || c.EmbeddingModel != "nomic-embed-text" ||
		c.SimilarityThreshold != 0.86 || c.Model != "claude-sonnet-5" ||
		c.ClaudeCmd != "claude" || c.MaxLogBytes != 65536 || c.Language != "English" {
		t.Errorf("defaults = %+v", c)
	}
}

func TestExplainSectionAbsentGetsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("defaultOrg: acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Explain.IsEnabled() || c.Explain.SimilarityThreshold != 0.86 {
		t.Errorf("explain = %+v", c.Explain)
	}
}

func TestExplainPartialSectionFillsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "explain:\n  enabled: false\n  model: claude-opus-4-8\n"
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Explain.IsEnabled() {
		t.Error("enabled: false must stick")
	}
	if c.Explain.Model != "claude-opus-4-8" {
		t.Errorf("model = %q", c.Explain.Model)
	}
	if c.Explain.OllamaURL != "http://localhost:11434" || c.Explain.MaxLogBytes != 65536 {
		t.Errorf("unset fields must get defaults: %+v", c.Explain)
	}
}

func TestResolveExplainStorePath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	p, err := ResolveExplainStorePath()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/xdg/ghrun/explain-db" {
		t.Errorf("path = %q", p)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -v -run TestExplain`
Expected: FAIL — `c.Explain undefined` (compile error).

- [ ] **Step 3: Write the implementation**

In `internal/config/config.go`:

Add the field to `Config`:

```go
// Config holds user settings for ghrun.
type Config struct {
	DefaultOrg             string        `yaml:"defaultOrg"`
	RefreshIntervalSeconds int           `yaml:"refreshIntervalSeconds"`
	RunListLimit           int           `yaml:"runListLimit"`
	ListPageSize           int           `yaml:"listPageSize"` // max rows shown at once in any list
	Favorites              []string      `yaml:"favorites"`    // "owner/name"
	Explain                ExplainConfig `yaml:"explain"`
}
```

Add after the `Config` type:

```go
// ExplainConfig configures the run-failure explanation feature (local RAG +
// Anthropic API / claude CLI). Every field is optional with a default.
type ExplainConfig struct {
	Enabled             *bool   `yaml:"enabled,omitempty"` // nil means enabled
	OllamaURL           string  `yaml:"ollamaURL"`
	EmbeddingModel      string  `yaml:"embeddingModel"`
	SimilarityThreshold float64 `yaml:"similarityThreshold"`
	AnthropicAPIKey     string  `yaml:"anthropicAPIKey"` // empty: read ANTHROPIC_API_KEY env
	Model               string  `yaml:"model"`
	ClaudeCmd           string  `yaml:"claudeCmd"`
	StorePath           string  `yaml:"storePath"` // empty: <config dir>/ghrun/explain-db
	MaxLogBytes         int     `yaml:"maxLogBytes"`
	Language            string  `yaml:"language"`
}

// IsEnabled reports whether explain is on. An unset flag means enabled, so
// existing config files without an explain section get the feature.
func (e ExplainConfig) IsEnabled() bool { return e.Enabled == nil || *e.Enabled }
```

Extend `Default()`:

```go
// Default returns the baseline configuration.
func Default() Config {
	return Config{
		RefreshIntervalSeconds: 4,
		RunListLimit:           30,
		ListPageSize:           20,
		Explain: ExplainConfig{
			OllamaURL:           "http://localhost:11434",
			EmbeddingModel:      "nomic-embed-text",
			SimilarityThreshold: 0.86,
			Model:               "claude-sonnet-5",
			ClaudeCmd:           "claude",
			MaxLogBytes:         65536,
			Language:            "English",
		},
	}
}
```

Extend `applyDefaults` (add before `return c`):

```go
	if c.Explain.OllamaURL == "" {
		c.Explain.OllamaURL = d.Explain.OllamaURL
	}
	if c.Explain.EmbeddingModel == "" {
		c.Explain.EmbeddingModel = d.Explain.EmbeddingModel
	}
	if c.Explain.SimilarityThreshold == 0 {
		c.Explain.SimilarityThreshold = d.Explain.SimilarityThreshold
	}
	if c.Explain.Model == "" {
		c.Explain.Model = d.Explain.Model
	}
	if c.Explain.ClaudeCmd == "" {
		c.Explain.ClaudeCmd = d.Explain.ClaudeCmd
	}
	if c.Explain.MaxLogBytes == 0 {
		c.Explain.MaxLogBytes = d.Explain.MaxLogBytes
	}
	if c.Explain.Language == "" {
		c.Explain.Language = d.Explain.Language
	}
```

Add after `ResolveCachePath`:

```go
// ResolveExplainStorePath returns the default explain knowledge-base
// directory (used when explain.storePath is empty).
func ResolveExplainStorePath() (string, error) {
	base, err := resolveBase("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ghrun", "explain-db"), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS — including the pre-existing config tests (the new `explain` block serializes alongside the old fields).

- [ ] **Step 5: Run the full suite and commit**

```bash
go test ./...
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): explain section with defaults

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 9: UI — messages, ExplainService interface, Explanation screen

**Files:**
- Modify: `internal/ui/client.go` (add `ExplainService`)
- Modify: `internal/ui/messages.go` (add explain messages + cmds)
- Create: `internal/ui/explain.go`
- Test: `internal/ui/explain_test.go`

**Interfaces:**
- Consumes: `explain.LocalResult`, `explain.ClaudeResult`, `explain.ExplainRequest` (Task 7); `GHClient.RunLogs`; viewport pattern from `logs.go`; `newLogs`, `pushMsg`, `errStyle`, `footerStyle`.
- Produces (used by Task 10):
  - `type ExplainService interface { ResolveLocal(ctx context.Context, logText string) (explain.LocalResult, error); AskClaude(ctx context.Context, req explain.ExplainRequest) (explain.ClaudeResult, error) }` in `client.go`
  - `newExplain(c GHClient, svc ExplainService, repo gh.RepoRef, id int64, detail gh.RunDetail) (*explainScreen, tea.Cmd)`
  - `type explainRunMsg struct { repo gh.RepoRef; id int64; detail gh.RunDetail }` in `messages.go`
  - test fake: `fakeExplainService` (pointer receivers) in `explain_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/explain_test.go`:

```go
package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// fakeExplainService also serves the app-level tests in Task 10.
type fakeExplainService struct {
	local     explain.LocalResult
	localErr  error
	claude    explain.ClaudeResult
	claudeErr error
	askCalls  int
	lastReq   explain.ExplainRequest
}

func (f *fakeExplainService) ResolveLocal(context.Context, string) (explain.LocalResult, error) {
	return f.local, f.localErr
}
func (f *fakeExplainService) AskClaude(_ context.Context, req explain.ExplainRequest) (explain.ClaudeResult, error) {
	f.askCalls++
	f.lastReq = req
	return f.claude, f.claudeErr
}

func failedDetail() gh.RunDetail {
	return gh.RunDetail{
		Run: gh.Run{ID: 5, Status: "completed", Conclusion: "failure", WorkflowName: "CI"},
		Jobs: []gh.Job{{
			Name: "build", Status: "completed", Conclusion: "failure",
			Steps: []gh.Step{
				{Name: "checkout", Status: "completed", Conclusion: "success"},
				{Name: "test", Status: "completed", Conclusion: "failure"},
			},
		}},
	}
}

func newTestExplain(svc ExplainService) *explainScreen {
	e, _ := newExplain(nil, svc, gh.RepoRef{Owner: "o", Name: "r"}, 5, failedDetail())
	return e
}

func TestExplainLogLoadedTriggersLocalSearch(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	s, cmd := e.Update(explainLogLoadedMsg{text: "Error: boom"})
	if cmd == nil {
		t.Fatal("log loaded must trigger the local search")
	}
	if !strings.Contains(s.View(), "Searching local knowledge") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainLogErrorShown(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	s, _ := e.Update(explainLogLoadedMsg{err: errorString("no logs available")})
	if !strings.Contains(s.View(), "no logs available") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainLocalHitShowsBadgeAndText(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	s, cmd := e.Update(explainLocalMsg{res: explain.LocalResult{
		Found: true, Explanation: "Root cause: OOM.", Similarity: 0.92,
	}})
	if cmd != nil {
		t.Error("a hit must not ask claude")
	}
	v := s.View()
	if !strings.Contains(v, "Root cause: OOM.") || !strings.Contains(v, "🧠 local · 92%") {
		t.Errorf("view = %q", v)
	}
}

func TestExplainExactHitBadge(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	s, _ := e.Update(explainLocalMsg{res: explain.LocalResult{Found: true, Exact: true, Explanation: "x", Similarity: 1}})
	if !strings.Contains(s.View(), "🧠 local · exact") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainMissAsksClaude(t *testing.T) {
	svc := &fakeExplainService{claude: explain.ClaudeResult{Explanation: "fresh", Source: "claude-sonnet-5"}}
	e := newTestExplain(svc)
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	s, cmd := e.Update(explainLocalMsg{res: explain.LocalResult{}})
	if cmd == nil {
		t.Fatal("a miss must ask claude")
	}
	if !strings.Contains(s.View(), "Asking claude") {
		t.Errorf("view = %q", s.View())
	}
	msg := cmd() // runs askClaudeCmd against the fake
	if svc.askCalls != 1 {
		t.Fatalf("askCalls = %d", svc.askCalls)
	}
	if svc.lastReq.Repo != "o/r" || svc.lastReq.Workflow != "CI" ||
		len(svc.lastReq.FailedSteps) != 1 || svc.lastReq.FailedSteps[0] != "build / test" {
		t.Errorf("req = %+v", svc.lastReq)
	}
	s, _ = s.Update(msg)
	v := s.View()
	if !strings.Contains(v, "fresh") || !strings.Contains(v, "✨ claude-sonnet-5") {
		t.Errorf("view = %q", v)
	}
}

func TestExplainRAGDisabledWarns(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	s, cmd := e.Update(explainLocalMsg{res: explain.LocalResult{RAGDisabled: true}})
	if cmd == nil {
		t.Fatal("RAG disabled must still ask claude")
	}
	if !strings.Contains(s.View(), "Ollama unreachable") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainClaudeErrorFallsBackToBestGuess(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	e.Update(explainLocalMsg{res: explain.LocalResult{Explanation: "old similar failure", Similarity: 0.55}})
	s, _ := e.Update(explainClaudeMsg{err: errorString("all explainers down")})
	v := s.View()
	if !strings.Contains(v, "old similar failure") || !strings.Contains(v, "best guess · 55%") {
		t.Errorf("view = %q", v)
	}
}

func TestExplainClaudeErrorWithoutGuess(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	e.Update(explainLocalMsg{res: explain.LocalResult{}})
	s, _ := e.Update(explainClaudeMsg{err: errorString("all explainers down")})
	v := s.View()
	if !strings.Contains(v, "all explainers down") || !strings.Contains(v, "l") {
		t.Errorf("view must show the error and point at raw logs: %q", v)
	}
}

func TestExplainRegenerateKey(t *testing.T) {
	svc := &fakeExplainService{claude: explain.ClaudeResult{Explanation: "v2", Source: "claude-sonnet-5"}}
	e := newTestExplain(svc)
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	e.Update(explainLocalMsg{res: explain.LocalResult{Found: true, Explanation: "v1", Similarity: 0.9}})
	s, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("'r' must regenerate")
	}
	if !strings.Contains(s.View(), "Asking claude") {
		t.Errorf("view = %q", s.View())
	}
}

func TestExplainLogsKeyPushesLogs(t *testing.T) {
	e := newTestExplain(&fakeExplainService{})
	e.Update(explainLogLoadedMsg{text: "Error: boom"})
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil {
		t.Fatal("'l' must push the logs screen")
	}
	if e.Title() != "explain" {
		t.Errorf("title = %q", e.Title())
	}
}
```

Note: `gh.Run` has a `WorkflowName` field (see `internal/gh/types.go:59`) — `failedDetail` uses it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -v -run TestExplain`
Expected: FAIL — `undefined: newExplain`, `undefined: explainLogLoadedMsg`, `undefined: ExplainService` (compile error).

- [ ] **Step 3: Add the ExplainService interface**

In `internal/ui/client.go`, extend the imports and append:

```go
import (
	"context"
	"time"

	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)
```

```go
// ExplainService is the subset of *explain.Service the UI depends on (for
// mockability, like GHClient).
type ExplainService interface {
	ResolveLocal(ctx context.Context, logText string) (explain.LocalResult, error)
	AskClaude(ctx context.Context, req explain.ExplainRequest) (explain.ClaudeResult, error)
}
```

- [ ] **Step 4: Add the messages and commands**

In `internal/ui/messages.go`, add `"context"`, `"fmt"`, `"strings"` and the explain import to the import block, then append:

```go
// --- explain feature ---

// explainRunMsg asks the App (which owns the explain service) to open the
// Explanation screen for a failed run.
type explainRunMsg struct {
	repo   gh.RepoRef
	id     int64
	detail gh.RunDetail
}

type explainLogLoadedMsg struct {
	text string
	err  error
}
type explainLocalMsg struct {
	res explain.LocalResult
	err error
}
type explainClaudeMsg struct {
	res explain.ClaudeResult
	err error
}

// loadExplainLogCmd fetches the failed log, falling back to the full log when
// the failed-only view is empty or unavailable.
func loadExplainLogCmd(c GHClient, repo gh.RepoRef, id int64) tea.Cmd {
	return func() tea.Msg {
		txt, err := c.RunLogs(repo, id, true)
		if err != nil || strings.TrimSpace(txt) == "" {
			txt, err = c.RunLogs(repo, id, false)
		}
		if err != nil {
			return explainLogLoadedMsg{err: err}
		}
		if strings.TrimSpace(txt) == "" {
			return explainLogLoadedMsg{err: fmt.Errorf("no logs available for run #%d", id)}
		}
		return explainLogLoadedMsg{text: txt}
	}
}

func resolveLocalCmd(svc ExplainService, logText string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.ResolveLocal(context.Background(), logText)
		return explainLocalMsg{res: res, err: err}
	}
}

func askClaudeCmd(svc ExplainService, req explain.ExplainRequest) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.AskClaude(context.Background(), req)
		return explainClaudeMsg{res: res, err: err}
	}
}
```

- [ ] **Step 5: Write the screen**

Create `internal/ui/explain.go`:

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// explainPhase tracks the screen's async pipeline: log → local RAG → claude.
type explainPhase int

const (
	phaseLoadingLog explainPhase = iota
	phaseSearching
	phaseAsking
	phaseDone
	phaseFailed
)

// explainScreen shows the explanation of a failed run (spec: écran Explanation).
type explainScreen struct {
	client GHClient
	svc    ExplainService
	repo   gh.RepoRef
	id     int64
	detail gh.RunDetail

	phase    explainPhase
	logText  string
	content  string // explanation currently shown
	badge    string // source badge (🧠 local / ✨ claude / best guess)
	warnText string // non-fatal warning (Ollama down, claude fallback…)
	errText  string
	guess    string  // best sub-threshold local match, shown if claude fails
	guessSim float32

	vp    viewport.Model
	ready bool
}

func newExplain(c GHClient, svc ExplainService, repo gh.RepoRef, id int64, detail gh.RunDetail) (*explainScreen, tea.Cmd) {
	e := &explainScreen{client: c, svc: svc, repo: repo, id: id, detail: detail, phase: phaseLoadingLog}
	return e, loadExplainLogCmd(c, repo, id)
}

func (e *explainScreen) Title() string { return "explain" }

// request builds the claude request from the run context and the full log.
func (e *explainScreen) request() explain.ExplainRequest {
	return explain.ExplainRequest{
		Log:         e.logText,
		Repo:        e.repo.String(),
		Workflow:    e.detail.WorkflowName,
		FailedSteps: failedSteps(e.detail),
	}
}

// setContent stores and (re)wraps the explanation into the viewport.
func (e *explainScreen) setContent(text string) {
	e.content = text
	if e.ready {
		e.vp.SetContent(lipgloss.NewStyle().Width(e.vp.Width).Render(text))
	}
}

func (e *explainScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case explainLogLoadedMsg:
		if m.err != nil {
			e.phase = phaseFailed
			e.errText = m.err.Error()
			return e, nil
		}
		e.logText = m.text
		e.phase = phaseSearching
		return e, resolveLocalCmd(e.svc, e.logText)

	case explainLocalMsg:
		if m.err != nil {
			e.warnText = "knowledge base unavailable: " + m.err.Error()
			e.phase = phaseAsking
			return e, askClaudeCmd(e.svc, e.request())
		}
		if m.res.RAGDisabled {
			e.warnText = "knowledge base disabled (Ollama unreachable)"
		}
		if m.res.Found {
			e.phase = phaseDone
			e.badge = localBadge(m.res)
			e.setContent(m.res.Explanation)
			return e, nil
		}
		e.guess, e.guessSim = m.res.Explanation, m.res.Similarity
		e.phase = phaseAsking
		return e, askClaudeCmd(e.svc, e.request())

	case explainClaudeMsg:
		if m.err != nil {
			if e.guess != "" {
				e.phase = phaseDone
				e.badge = fmt.Sprintf("🧠 best guess · %.0f%%", e.guessSim*100)
				e.warnText = "claude unavailable: " + m.err.Error()
				e.setContent(e.guess)
				return e, nil
			}
			e.phase = phaseFailed
			e.errText = m.err.Error() + " — press l to open the raw logs"
			return e, nil
		}
		e.phase = phaseDone
		e.badge = "✨ " + m.res.Source
		e.setContent(m.res.Explanation)
		return e, nil

	case tea.WindowSizeMsg:
		if !e.ready {
			e.vp = viewport.New(m.Width, max(3, m.Height-8))
			e.ready = true
		} else {
			e.vp.Width = m.Width
			e.vp.Height = max(3, m.Height-8)
		}
		e.setContent(e.content)
		return e, nil

	case tea.KeyMsg:
		switch m.String() {
		case "r":
			if e.logText != "" && e.phase != phaseAsking {
				e.phase = phaseAsking
				e.badge = ""
				return e, askClaudeCmd(e.svc, e.request())
			}
			return e, nil
		case "l":
			lg, cmd := newLogs(e.client, e.repo, e.id, true)
			return e, tea.Batch(func() tea.Msg { return pushMsg{screen: lg} }, cmd)
		}
		var cmd tea.Cmd
		e.vp, cmd = e.vp.Update(msg)
		return e, cmd

	case tea.MouseMsg:
		var cmd tea.Cmd
		e.vp, cmd = e.vp.Update(msg)
		return e, cmd
	}
	return e, nil
}

func (e *explainScreen) View() string {
	var b strings.Builder
	header := fmt.Sprintf("run #%d — %s", e.id, e.repo.String())
	if e.badge != "" {
		header += "   " + e.badge
	}
	b.WriteString(header + "\n")
	if e.warnText != "" {
		b.WriteString(footerStyle.Render("⚠ "+e.warnText) + "\n")
	}
	b.WriteString("\n")
	switch e.phase {
	case phaseLoadingLog:
		b.WriteString("Fetching failed log…")
	case phaseSearching:
		b.WriteString("Searching local knowledge…")
	case phaseAsking:
		b.WriteString("Asking claude… (may take a minute)")
	case phaseFailed:
		b.WriteString(errStyle.Render("⚠ " + e.errText))
	case phaseDone:
		if e.ready {
			b.WriteString(e.vp.View())
		} else {
			b.WriteString(e.content)
		}
	}
	b.WriteString("\n\n" + footerStyle.Render("r regenerate · l raw logs"))
	return b.String()
}

// localBadge renders the knowledge-base hit badge. %.0f rounds instead of
// truncating (float32 0.92*100 can land just below 92).
func localBadge(res explain.LocalResult) string {
	if res.Exact {
		return "🧠 local · exact"
	}
	return fmt.Sprintf("🧠 local · %.0f%%", res.Similarity*100)
}

// failedSteps lists "job / step" pairs whose conclusion is failure.
func failedSteps(d gh.RunDetail) []string {
	var out []string
	for _, j := range d.Jobs {
		for _, s := range j.Steps {
			if s.Conclusion == "failure" {
				out = append(out, j.Name+" / "+s.Name)
			}
		}
	}
	return out
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v -run TestExplain`
Expected: PASS (11 tests).

- [ ] **Step 7: Run the full suite and commit**

```bash
go test ./...
git add internal/ui/client.go internal/ui/messages.go internal/ui/explain.go internal/ui/explain_test.go
git commit -m "feat(ui): explanation screen with local RAG and claude phases

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 10: Wire `e` into run detail and the App shell

**Files:**
- Modify: `internal/ui/rundetail.go`
- Modify: `internal/ui/app.go`
- Test: `internal/ui/rundetail_test.go` (append), `internal/ui/app_test.go` (append)

**Interfaces:**
- Consumes: `explainRunMsg`, `newExplain`, `ExplainService` (Task 9); test helpers `fakeExplainService` and `failedDetail()` defined in `internal/ui/explain_test.go` (Task 9, same package).
- Produces: `rundetail.explainAvailable bool` (set by the App on push), `App.WithExplainService(s ExplainService) App`. The App handles `explainRunMsg` by pushing the explain screen — `runs.go` and `launch.go` stay untouched.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/rundetail_test.go`:

```go
func TestRunDetailExplainKeyEmitsMsg(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.explainAvailable = true
	rd.Update(runDetailLoadedMsg{detail: failedDetail()})
	_, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd == nil {
		t.Fatal("'e' on a failed run must emit explainRunMsg")
	}
	msg, ok := cmd().(explainRunMsg)
	if !ok {
		t.Fatalf("msg = %T", cmd())
	}
	if msg.id != 5 || msg.repo.Owner != "o" || msg.detail.WorkflowName != "CI" {
		t.Errorf("msg = %+v", msg)
	}
}

func TestRunDetailExplainKeyInactiveOnSuccess(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.explainAvailable = true
	rd.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 5, Status: "completed", Conclusion: "success"}}})
	if _, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}); cmd != nil {
		t.Error("'e' must be inactive on a successful run")
	}
}

func TestRunDetailExplainKeyInactiveWithoutService(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.Update(runDetailLoadedMsg{detail: failedDetail()})
	if _, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}); cmd != nil {
		t.Error("'e' must be inactive when explain is disabled")
	}
}

func TestRunDetailFooterExplainHint(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.explainAvailable = true
	s, _ := rd.Update(runDetailLoadedMsg{detail: failedDetail()})
	if !strings.Contains(s.View(), "e explain") {
		t.Errorf("failed run must hint 'e explain':\n%s", s.View())
	}
	rd2, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 6)
	rd2.explainAvailable = true
	s2, _ := rd2.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 6, Status: "completed", Conclusion: "success"}}})
	if strings.Contains(s2.View(), "e explain") {
		t.Error("successful run must not hint explain")
	}
	rd3, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 7)
	s3, _ := rd3.Update(runDetailLoadedMsg{detail: failedDetail()})
	if strings.Contains(s3.View(), "e explain") {
		t.Error("disabled explain must not hint")
	}
}
```

Append to `internal/ui/app_test.go`:

```go
func TestPushMarksRunDetailExplainAvailable(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithExplainService(&fakeExplainService{})
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 1)
	m, _ := a.Update(pushMsg{screen: rd})
	a2 := m.(App)
	got, ok := a2.top().(*rundetail)
	if !ok {
		t.Fatalf("top = %T, want *rundetail", a2.top())
	}
	if !got.explainAvailable {
		t.Error("pushed rundetail must have explainAvailable set")
	}

	b := NewApp(nil, config.Config{DefaultOrg: "acme"})
	rd2, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 2)
	m2, _ := b.Update(pushMsg{screen: rd2})
	if m2.(App).top().(*rundetail).explainAvailable {
		t.Error("no service: explainAvailable must stay false")
	}
}

func TestExplainRunMsgPushesExplainScreen(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithExplainService(&fakeExplainService{})
	m, cmd := a.Update(explainRunMsg{repo: gh.RepoRef{Owner: "o", Name: "r"}, id: 5})
	if cmd == nil {
		t.Error("pushing explain must start the log fetch")
	}
	if _, ok := m.(App).top().(*explainScreen); !ok {
		t.Fatalf("top = %T", m.(App).top())
	}
}

func TestExplainRunMsgIgnoredWithoutService(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"})
	before := len(a.stack)
	m, _ := a.Update(explainRunMsg{repo: gh.RepoRef{Owner: "o", Name: "r"}, id: 5})
	if len(m.(App).stack) != before {
		t.Error("explainRunMsg without a service must be a no-op")
	}
}
```

(Ensure `app_test.go` imports `github.com/stephaneHerraiz/ghrun/internal/gh` — add it if missing.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -v -run 'TestRunDetailExplain|TestRunDetailFooter|TestPushMarks|TestExplainRunMsg'`
Expected: FAIL — `rd.explainAvailable undefined`, `a.WithExplainService undefined` (compile error).

- [ ] **Step 3: Modify rundetail**

In `internal/ui/rundetail.go`:

Add the field to the struct:

```go
type rundetail struct {
	client   GHClient
	repo     gh.RepoRef
	id       int64
	detail   gh.RunDetail
	loaded   bool
	interval time.Duration
	// explainAvailable is stamped by the App on push: the explain service is
	// configured and enabled.
	explainAvailable bool
}
```

Add the eligibility helper after `active()`:

```go
// explainable reports whether the run's conclusion warrants an explanation.
func (d *rundetail) explainable() bool {
	return d.loaded && (d.detail.Conclusion == "failure" || d.detail.Conclusion == "timed_out")
}
```

Add the key in the `tea.KeyMsg` switch (after `case "o":`):

```go
		case "e":
			if d.explainAvailable && d.explainable() {
				detail := d.detail
				return d, func() tea.Msg { return explainRunMsg{repo: d.repo, id: d.id, detail: detail} }
			}
```

Replace the footer line in `View()`:

```go
	hints := "[l] logs  ·  r rerun  f rerun-failed  x cancel  o web"
	if d.explainAvailable && d.explainable() {
		hints += "  ·  e explain"
	}
	b.WriteString("\n" + hints)
```

- [ ] **Step 4: Modify the App shell**

In `internal/ui/app.go`:

Add the field to `App`:

```go
type App struct {
	client     GHClient
	cfg        config.Config
	stack      []Screen
	repo       *gh.RepoRef // current repo context (nil at dashboard)
	width      int
	height     int
	errText    string
	showHelp   bool
	saveConfig func(config.Config) error
	explainSvc ExplainService // nil when explain is disabled or unconfigured
}
```

Add after `NewApp`:

```go
// WithExplainService enables the run-failure explanation feature. Screens
// never hold the service themselves: rundetail emits explainRunMsg and the
// App resolves it here.
func (a App) WithExplainService(s ExplainService) App {
	a.explainSvc = s
	return a
}
```

In `Update`, replace the `pushMsg` case:

```go
	case pushMsg:
		// Stamp feature availability on screens that surface it contextually.
		if rd, ok := m.screen.(*rundetail); ok {
			rd.explainAvailable = a.explainSvc != nil
		}
		a.push(m.screen)
		return a, nil
	case explainRunMsg:
		if a.explainSvc == nil {
			return a, nil
		}
		es, cmd := newExplain(a.client, a.explainSvc, m.repo, m.id, m.detail)
		a.push(es)
		return a, cmd
```

In `footer()`, update the expanded help's Runs line:

```go
			"Runs: r rerun · f rerun-failed · x cancel · o open web · l logs · e explain · g refresh",
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS — including the pre-existing rundetail/app tests.

- [ ] **Step 6: Run the full suite and commit**

```bash
go test ./...
git add internal/ui/rundetail.go internal/ui/app.go internal/ui/rundetail_test.go internal/ui/app_test.go
git commit -m "feat(ui): wire explain into run detail and app shell

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 11: Bootstrap wiring, README, smoke test

**Files:**
- Modify: `cmd/ghrun/bootstrap.go`
- Test: `cmd/ghrun/bootstrap_test.go` (append)
- Modify: `README.md`

**Interfaces:**
- Consumes: `config.ExplainConfig.IsEnabled`, `config.ResolveExplainStorePath` (Task 8); `explain.NewOllamaEmbedder`, `explain.NewChromemStore`, `explain.NewAnthropicExplainer`, `explain.NewClaudeCLIExplainer`, `explain.Chain`, `explain.NewService`, `explain.Options` (Tasks 2–7); `ui.App.WithExplainService` (Task 10).
- Produces: `buildExplainService(cfg config.Config) ui.ExplainService` — returns nil when disabled.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/ghrun/bootstrap_test.go`:

```go
func TestBuildExplainServiceDisabled(t *testing.T) {
	off := false
	cfg := config.Default()
	cfg.Explain.Enabled = &off
	if svc := buildExplainService(cfg); svc != nil {
		t.Error("disabled explain must yield a nil service")
	}
}

func TestBuildExplainServiceEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Explain.StorePath = t.TempDir() // never touch the real config dir
	svc := buildExplainService(cfg)
	if svc == nil {
		t.Fatal("enabled explain must yield a service")
	}
}
```

(Ensure the test file imports `github.com/stephaneHerraiz/ghrun/internal/config`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ghrun/ -v -run TestBuildExplain`
Expected: FAIL — `undefined: buildExplainService` (compile error).

- [ ] **Step 3: Write the bootstrap wiring**

In `cmd/ghrun/bootstrap.go`, add `"os/exec"` and the explain import, then append:

```go
// buildExplainService assembles the run-failure explanation service from
// config, or nil when the feature is disabled. Explainer order encodes the
// fallback: Anthropic API when a key is configured, then the claude CLI when
// the binary is on PATH. Every part degrades gracefully at runtime, so
// construction never fails.
func buildExplainService(cfg config.Config) ui.ExplainService {
	ec := cfg.Explain
	if !ec.IsEnabled() {
		return nil
	}
	storePath := ec.StorePath
	if storePath == "" {
		if p, err := config.ResolveExplainStorePath(); err == nil {
			storePath = p
		}
	}
	var store explain.Store
	if storePath != "" {
		if s, err := explain.NewChromemStore(storePath); err == nil {
			store = s
		}
	}
	var explainers []explain.Explainer
	key := ec.AnthropicAPIKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if key != "" {
		explainers = append(explainers, explain.NewAnthropicExplainer(key, ec.Model))
	}
	if _, err := exec.LookPath(ec.ClaudeCmd); err == nil {
		explainers = append(explainers, explain.NewClaudeCLIExplainer(ec.ClaudeCmd))
	}
	return explain.NewService(
		explain.NewOllamaEmbedder(ec.OllamaURL, ec.EmbeddingModel),
		&explain.Chain{Explainers: explainers},
		store,
		explain.Options{
			Threshold:   float32(ec.SimilarityThreshold),
			MaxLogBytes: ec.MaxLogBytes,
			Language:    ec.Language,
		},
	)
}
```

And wire it in `run()`:

```go
	app := ui.NewApp(client, cfg)
	if svc := buildExplainService(cfg); svc != nil {
		app = app.WithExplainService(svc)
	}
	_, err = tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ghrun/ -v`
Expected: PASS.

- [ ] **Step 5: Update the README**

In `README.md`:

1. **Screens** table (after the Logs row):

```markdown
| **Explanation** | Why a failed run failed: instant from the local knowledge base, or generated by Claude and memorized. |
```

2. **Keybindings → Run detail** table:

```markdown
| `l` | View logs |
| `e` | Explain the failure (failed/timed-out runs only) |
| `r` | Rerun · `f` rerun-failed · `x` cancel · `o` open web |
```

3. New **Keybindings → Explanation** section after Logs:

```markdown
### Explanation

| Key | Action |
|---|---|
| `r` | Regenerate with Claude (updates the knowledge base) |
| `l` | Open the raw failed logs |
| `↑`/`↓` · `PgUp`/`PgDn` · mouse wheel | Scroll |
```

4. **Features** list (new bullet):

```markdown
- **Failure explanations (RAG + Claude)**: `e` on a failed run explains the
  root cause. Errors already seen are answered instantly and offline from a
  local knowledge base (Ollama embeddings + chromem-go); new ones are explained
  by Claude (Anthropic API if `ANTHROPIC_API_KEY` is set, otherwise the
  `claude` CLI) and memorized. Fully optional: without Ollama and Claude the
  rest of ghrun is unaffected.
```

5. **Configuration**: extend the YAML example and the key table:

```yaml
explain:                          # run-failure explanations (all optional)
  enabled: true
  ollamaURL: http://localhost:11434
  embeddingModel: nomic-embed-text
  similarityThreshold: 0.86       # cosine similarity for a local hit
  anthropicAPIKey: ""             # empty: read ANTHROPIC_API_KEY env
  model: claude-sonnet-5
  claudeCmd: claude               # CLI fallback when no API key
  storePath: ~/.config/ghrun/explain-db
  maxLogBytes: 65536              # log truncated from the end
  language: English
```

```markdown
| `explain.enabled` | `true` | Toggle the failure-explanation feature. |
| `explain.ollamaURL` | `http://localhost:11434` | Local Ollama server for embeddings. |
| `explain.embeddingModel` | `nomic-embed-text` | Ollama embedding model. |
| `explain.similarityThreshold` | `0.86` | Cosine similarity for a knowledge-base hit. |
| `explain.anthropicAPIKey` | *(empty)* | Anthropic API key; falls back to `ANTHROPIC_API_KEY`. |
| `explain.model` | `claude-sonnet-5` | Model used by the API explainer. |
| `explain.claudeCmd` | `claude` | CLI binary used when no API key is configured. |
| `explain.storePath` | `~/.config/ghrun/explain-db` | Knowledge-base directory. |
| `explain.maxLogBytes` | `65536` | Max raw-log bytes sent to Claude (truncated from the end). |
| `explain.language` | `English` | Language of generated explanations. |
```

6. **Requirements**: add an optional bullet:

```markdown
- **Optional, for failure explanations**: a local [Ollama](https://ollama.com)
  with `ollama pull nomic-embed-text` (knowledge-base search), plus either an
  `ANTHROPIC_API_KEY` or the [`claude` CLI](https://claude.com/claude-code)
  (generation). Missing pieces degrade gracefully.
```

- [ ] **Step 6: Full verification**

```bash
go build ./... && go vet ./... && go test ./...
```
Expected: everything green.

Manual smoke test (requires a repo with a failed run; Ollama/claude optional — degradation paths are part of the test):

```bash
go run ./cmd/ghrun
# → open a failed run → 'e' → observe: fetching → searching → asking claude →
#   explanation with a ✨ badge. Press esc, 'e' again → instant 🧠 local · exact.
```

- [ ] **Step 7: Commit**

```bash
git add cmd/ghrun/bootstrap.go cmd/ghrun/bootstrap_test.go README.md
git commit -m "feat: bootstrap explain service and document the feature

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Spec deviations (conscious, minor)

- `types.go` is added to the spec's file list (interfaces, shared types, prompts) — the spec did not assign them a file.
- The `Explainer` interface gains a `Name() string` method beyond the spec's `Explain(ctx, req) (string, error)`: the UI badge (`✨ claude-sonnet-5` vs `✨ claude (cli)`) needs each explainer to identify itself.
- `chain.go` returns a `ChainResult{Explanation, Source}` instead of implementing the `Explainer` interface itself: the UI badge needs to know **which** explainer answered, which a bare `(string, error)` cannot convey.
- `Entry` carries its `Embedding` so `Store.Upsert(e Entry)` can keep the spec's single-argument signature.
- `explain.enabled` uses a `*bool` (`nil` = enabled) so existing config files without the section get the feature without edits.
- `r` (regenerate) only fires from a terminal phase (`phaseDone`/`phaseFailed`) instead of the spec's « quel que soit l'état courant » : an any-state regenerate races the in-flight async pipeline (a stale local result could clobber the fresh claude answer). The pipeline is strictly sequential by design since the Task 9 review.
