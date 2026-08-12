# Chat About a Run With Claude — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the run-detail screen of any run, key `c` writes the run log and the workflow YAML (as it was at the run's commit) to a context directory, then hands the terminal over to the interactive `claude` CLI — started inside the repository's local clone when one is found — with a prompt that points at both files.

**Architecture:** A new `internal/chat` package holds all the pure logic (materialising the context on disk, rendering the initial prompt, locating the local clone, building the `*exec.Cmd`) with no Bubbletea dependency, mirroring how `internal/explain` is kept free of UI concerns. `internal/gh` gains two calls: `RunMeta` (one `gh api repos/{o}/{r}/actions/runs/{id}` request — the only source of the workflow **file path**, which `gh run view --json` does not expose) and `WorkflowFile` (contents endpoint at a given ref), with the existing `WorkflowInputs` refactored on top of the latter. In the UI, `rundetail` emits `chatRequestMsg` on `c`; the root `App` resolves it into an async `prepareChatCmd`, then suspends its ticker and returns `tea.ExecProcess`, resuming on `chatDoneMsg`.

**Tech Stack:** Go 1.25, Bubbletea v1.3.10 (`tea.ExecProcess`), the `gh` CLI via the existing `gh.Runner` abstraction. **No new dependencies.**

**Spec:** `docs/superpowers/specs/2026-08-10-chat-run-context-design.md`

## Global Constraints

- Branch: `feat/chat-run-context` (already checked out, spec committed as `f595945`).
- All code, identifiers, comments, UI copy, commit messages and the generated Claude prompt: **English**. (Only this plan and the spec are in French.)
- **No new Go dependencies.** Everything uses the standard library plus what `go.mod` already pins.
- Defaults, verbatim: `chat.enabled` = `true` (absent means enabled), `chat.claudeCmd` = `"claude"`, `chat.cloneRoots` = `["~/dev"]`.
- Context directory: `<XDG_CACHE_HOME|~/.cache>/ghrun/chat/<owner>/<name>/<runID>/`, files `log.txt` and `workflow.yml`.
- Contextual screen keys are lowercase (project convention). `c` is free on run detail (`l`, `e`, `r`, `f`, `x`, `o` are taken).
- Every chat failure is **non-fatal**: it surfaces through the existing `errMsg` → red footer path. Only `AuthStatus` at startup is fatal.
- **No test may require a real `claude` binary, a real `gh`, network access, or a TTY.** `chat.Command` returns an `*exec.Cmd` that tests inspect without running.
- Run `go test ./...` (all packages) before every commit — the whole suite must stay green.
- End every commit message with:
  `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`

## Verified facts (do not re-derive)

These were checked against the live `gh` CLI and the module cache while writing this plan.

- `gh run view --json` accepts: `attempt, conclusion, createdAt, databaseId, displayTitle, event, headBranch, headSha, jobs, name, number, startedAt, status, updatedAt, url, workflowDatabaseId, workflowName`. It **does not** expose the workflow file path — hence `RunMeta` goes through `gh api`.
- `gh api repos/{owner}/{name}/actions/runs/{id}` returns, among others:
  `{"name": "...", "display_title": "...", "run_number": 1419, "path": ".github/workflows/merge-release.yaml", "head_sha": "df53ccd…", "head_branch": "master", "status": "completed", "conclusion": "skipped", "html_url": "https://github.com/…/actions/runs/31484657511"}`.
  `name` is the workflow run's name (the workflow's `name:` unless `run-name:` overrides it).
- `gh api repos/{owner}/{name}/contents/{path}?ref={sha} --jq .content` returns line-wrapped base64. GitHub wraps with `\n`; some proxies inject `\r\n`, so both must be stripped before decoding (the existing `WorkflowInputs` already does this).
- `tea.ExecProcess(c *exec.Cmd, fn ExecCallback) Cmd` exists in `bubbletea v1.3.10` (`exec.go:50`). `ExecCallback` is `func(error) tea.Msg`. Bubbletea releases the terminal for the duration and restores it after.
- The `claude` CLI starts an **interactive** session when given a bare prompt argument (`claude [options] [prompt]`); `-p/--print` is what makes it non-interactive. `--add-dir <directories...>` grants tool access to directories outside the working directory.
- `internal/ui` tests never build a fake `GHClient` — they pass `nil` and exercise pure view/update logic. Task 6 therefore introduces a **narrow** `chatFetcher` interface (3 methods) so its test fake stays small.
- `internal/gh` tests use `fakeRunner` from `fixtures_test.go`: `(&fakeRunner{}).push(out string, err error)` queues responses in order, `f.lastCall() []string` returns the last argv.

## File Structure

```
internal/config/
├── config.go               # Task 1 — ChatConfig, ExpandHome, ResolveChatCacheDir
└── config_test.go          # Task 1

internal/gh/
├── client_runmeta.go       # Task 2 — RunMeta (new file)
├── client_runmeta_test.go  # Task 2
├── client_inputs.go        # Task 2 — WorkflowFile + WorkflowInputs refactored onto it
└── client_inputs_test.go   # Task 2

internal/chat/              # new package, zero Bubbletea, zero gh
├── types.go                # Task 3 — Context, Session, ErrNoContext
├── prepare.go              # Task 3 — Prepare
├── prepare_test.go         # Task 3
├── prompt.go               # Task 4 — Prompt
├── prompt_test.go          # Task 4
├── launch.go               # Task 5 — FindClone, Command
└── launch_test.go          # Task 5

internal/ui/
├── messages.go             # Task 6 — chatFetcher, chat messages, prepareChatCmd
├── chat_test.go            # Task 6 — prepareChatCmd tests
├── client.go               # Task 6 — GHClient gains RunMeta + WorkflowFile
├── rundetail.go            # Task 7 — `c` key, chatAvailable, footer hint
├── rundetail_test.go       # Task 7
├── app.go                  # Task 7 — WithChatCacheDir, suspended, ExecProcess wiring
└── app_test.go             # Task 7

cmd/ghrun/
└── bootstrap.go            # Task 8 — wire the cache dir, drop the private expandHome

README.md                   # Task 8 — keybinding, feature, config docs
```

---

### Task 1: Config — `chat` section, `ExpandHome`, cache dir

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing (leaf package).
- Produces:
  - `type ChatConfig struct { Enabled *bool; ClaudeCmd string; CloneRoots []string }`
  - `func (c ChatConfig) IsEnabled() bool`
  - `Config.Chat ChatConfig` (yaml key `chat`)
  - `func ExpandHome(p string) string`
  - `func ResolveChatCacheDir() (string, error)`

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
func TestChatDefaults(t *testing.T) {
	d := Default()
	if d.Chat.ClaudeCmd != "claude" {
		t.Errorf("ClaudeCmd = %q, want claude", d.Chat.ClaudeCmd)
	}
	if len(d.Chat.CloneRoots) != 1 || d.Chat.CloneRoots[0] != "~/dev" {
		t.Errorf("CloneRoots = %v, want [~/dev]", d.Chat.CloneRoots)
	}
	if !d.Chat.IsEnabled() {
		t.Error("chat should be enabled by default")
	}
}

func TestChatDisabledExplicitly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("chat:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Chat.IsEnabled() {
		t.Error("chat.enabled: false must disable the feature")
	}
	// Other chat fields still get their defaults.
	if c.Chat.ClaudeCmd != "claude" {
		t.Errorf("ClaudeCmd = %q", c.Chat.ClaudeCmd)
	}
}

func TestChatSectionAbsentMeansEnabledWithDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("defaultOrg: acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Chat.IsEnabled() {
		t.Error("a config with no chat section must keep chat enabled")
	}
	if len(c.Chat.CloneRoots) != 1 || c.Chat.CloneRoots[0] != "~/dev" {
		t.Errorf("CloneRoots = %v", c.Chat.CloneRoots)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := ExpandHome("~/dev"); got != filepath.Join(home, "dev") {
		t.Errorf("ExpandHome(~/dev) = %q", got)
	}
	if got := ExpandHome("/abs/path"); got != "/abs/path" {
		t.Errorf("absolute path must be untouched, got %q", got)
	}
	if got := ExpandHome(""); got != "" {
		t.Errorf("empty must stay empty, got %q", got)
	}
	// A bare "~" is not expanded: only the "~/" prefix is documented.
	if got := ExpandHome("~weird"); got != "~weird" {
		t.Errorf("~weird = %q", got)
	}
}

func TestResolveChatCacheDirHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdgcache")
	p, err := ResolveChatCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join("/tmp/xdgcache", "ghrun", "chat") {
		t.Errorf("dir = %q", p)
	}
}
```

Check the imports at the top of `config_test.go` include `os` and `path/filepath`; add them if missing.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'Chat|ExpandHome' -v`
Expected: FAIL — `undefined: ExpandHome`, `d.Chat undefined`, `undefined: ResolveChatCacheDir`.

- [ ] **Step 3: Implement**

In `internal/config/config.go`, add the field to `Config` (after `Explain`):

```go
	Chat                   ChatConfig    `yaml:"chat"`
```

Add the type after `ExplainConfig`'s `IsEnabled`:

```go
// ChatConfig configures the "discuss this run with Claude" feature. It is
// deliberately independent of ExplainConfig: chat must work even when the
// explanation feature is disabled.
type ChatConfig struct {
	Enabled    *bool    `yaml:"enabled,omitempty"` // nil means enabled
	ClaudeCmd  string   `yaml:"claudeCmd"`
	CloneRoots []string `yaml:"cloneRoots"` // where to look for the repo's local clone
}

// IsEnabled reports whether chat is on. An unset flag means enabled, so
// existing config files without a chat section get the feature.
func (c ChatConfig) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }
```

In `Default()`, add after the `Explain:` block:

```go
		Chat: ChatConfig{
			ClaudeCmd:  "claude",
			CloneRoots: []string{"~/dev"},
		},
```

In `applyDefaults`, add before `return c`:

```go
	if c.Chat.ClaudeCmd == "" {
		c.Chat.ClaudeCmd = d.Chat.ClaudeCmd
	}
	if len(c.Chat.CloneRoots) == 0 {
		c.Chat.CloneRoots = d.Chat.CloneRoots
	}
```

Add the two helpers next to the other `Resolve*` functions:

```go
// ExpandHome resolves a leading "~/" so documented paths like "~/dev" work
// verbatim; callers would otherwise create or stat a literal "~" directory.
func ExpandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// ResolveChatCacheDir returns the base directory where per-run chat context
// files are written.
func ResolveChatCacheDir() (string, error) {
	base, err := resolveBase("XDG_CACHE_HOME", ".cache")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ghrun", "chat"), nil
}
```

Add `"strings"` to the import block of `config.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all tests, including the pre-existing ones).

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add chat section, ExpandHome and the chat cache dir

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: gh — `RunMeta` and `WorkflowFile`

**Files:**
- Create: `internal/gh/client_runmeta.go`
- Create: `internal/gh/client_runmeta_test.go`
- Modify: `internal/gh/client_inputs.go`
- Modify: `internal/gh/client_inputs_test.go`
- Modify: `internal/gh/types.go` (add `RunMeta`)

**Interfaces:**
- Consumes: `Client`, `Runner`, `RepoRef` (existing).
- Produces:
  - `type RunMeta struct { WorkflowName, WorkflowPath, HeadSHA, HeadBranch string; Number int; Status, Conclusion, WebURL string }`
  - `func (c *Client) RunMeta(repo RepoRef, id int64) (RunMeta, error)`
  - `func (c *Client) WorkflowFile(repo RepoRef, path, ref string) ([]byte, error)`

- [ ] **Step 1: Write the failing tests**

Create `internal/gh/client_runmeta_test.go`:

```go
package gh

import (
	"errors"
	"testing"
)

const sampleRunMetaJSON = `{
  "name": "Build & test",
  "run_number": 1419,
  "path": ".github/workflows/ci.yaml",
  "head_sha": "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0",
  "head_branch": "feature/x",
  "status": "completed",
  "conclusion": "failure",
  "html_url": "https://github.com/o/r/actions/runs/4211"
}`

func TestRunMetaParses(t *testing.T) {
	f := (&fakeRunner{}).push(sampleRunMetaJSON, nil)
	c := NewClient(f)

	m, err := c.RunMeta(RepoRef{"o", "r"}, 4211)
	if err != nil {
		t.Fatal(err)
	}
	if m.WorkflowName != "Build & test" || m.WorkflowPath != ".github/workflows/ci.yaml" {
		t.Errorf("workflow = %q / %q", m.WorkflowName, m.WorkflowPath)
	}
	if m.HeadSHA != "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0" || m.HeadBranch != "feature/x" {
		t.Errorf("head = %q / %q", m.HeadSHA, m.HeadBranch)
	}
	if m.Number != 1419 || m.Status != "completed" || m.Conclusion != "failure" {
		t.Errorf("meta = %+v", m)
	}
	if m.WebURL != "https://github.com/o/r/actions/runs/4211" {
		t.Errorf("WebURL = %q", m.WebURL)
	}

	got := f.lastCall()
	if len(got) != 2 || got[0] != "api" || got[1] != "repos/o/r/actions/runs/4211" {
		t.Errorf("argv = %v", got)
	}
}

func TestRunMetaPropagatesRunnerError(t *testing.T) {
	boom := errors.New("boom")
	c := NewClient((&fakeRunner{}).push("", boom))
	if _, err := c.RunMeta(RepoRef{"o", "r"}, 1); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestRunMetaRejectsGarbage(t *testing.T) {
	c := NewClient((&fakeRunner{}).push("not json", nil))
	if _, err := c.RunMeta(RepoRef{"o", "r"}, 1); err == nil {
		t.Fatal("expected a parse error")
	}
}
```

Append to `internal/gh/client_inputs_test.go`:

```go
func TestWorkflowFileDecodesAndPassesRef(t *testing.T) {
	// GitHub line-wraps base64 with \n; some proxies inject \r\n.
	wrapped := b64(sampleWorkflow)[:10] + "\r\n" + b64(sampleWorkflow)[10:]
	f := (&fakeRunner{}).push(wrapped+"\n", nil)
	c := NewClient(f)

	out, err := c.WorkflowFile(RepoRef{"o", "r"}, ".github/workflows/deploy.yml", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != sampleWorkflow {
		t.Fatalf("content = %q", string(out))
	}
	got := f.lastCall()
	if got[1] != "repos/o/r/contents/.github/workflows/deploy.yml?ref=abc123" {
		t.Errorf("argv = %v", got)
	}
}

func TestWorkflowFileOmitsEmptyRef(t *testing.T) {
	f := (&fakeRunner{}).push(b64(sampleWorkflow), nil)
	c := NewClient(f)
	if _, err := c.WorkflowFile(RepoRef{"o", "r"}, ".github/workflows/deploy.yml", ""); err != nil {
		t.Fatal(err)
	}
	if got := f.lastCall(); got[1] != "repos/o/r/contents/.github/workflows/deploy.yml" {
		t.Errorf("argv = %v", got)
	}
}

func TestWorkflowFileRejectsBadBase64(t *testing.T) {
	c := NewClient((&fakeRunner{}).push("!!!not base64!!!", nil))
	if _, err := c.WorkflowFile(RepoRef{"o", "r"}, "p", ""); err == nil {
		t.Fatal("expected a decode error")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/gh/ -run 'RunMeta|WorkflowFile' -v`
Expected: FAIL — `c.RunMeta undefined`, `c.WorkflowFile undefined`.

- [ ] **Step 3: Implement `RunMeta`**

Add to `internal/gh/types.go`, after `RunDetail`:

```go
// RunMeta is run metadata that `gh run view --json` does not expose — chiefly
// the workflow file path — fetched from the REST API in a single call.
type RunMeta struct {
	WorkflowName string
	WorkflowPath string // ".github/workflows/ci.yaml"
	HeadSHA      string
	HeadBranch   string
	Number       int
	Status       string
	Conclusion   string
	WebURL       string
}
```

Create `internal/gh/client_runmeta.go`:

```go
package gh

import (
	"encoding/json"
	"fmt"
)

// jsonRunMeta mirrors the subset of GET /repos/{o}/{r}/actions/runs/{id} we use.
type jsonRunMeta struct {
	Name       string `json:"name"`
	Number     int    `json:"run_number"`
	Path       string `json:"path"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

// RunMeta returns the run's workflow path, head commit and display metadata.
// It goes through `gh api` rather than `gh run view --json` because the
// workflow *file path* is not among the fields that command can return.
func (c *Client) RunMeta(repo RepoRef, id int64) (RunMeta, error) {
	out, err := c.run.Exec("api",
		fmt.Sprintf("repos/%s/%s/actions/runs/%d", repo.Owner, repo.Name, id))
	if err != nil {
		return RunMeta{}, err
	}
	var raw jsonRunMeta
	if err := json.Unmarshal(out, &raw); err != nil {
		return RunMeta{}, fmt.Errorf("parsing run meta: %w", err)
	}
	return RunMeta{
		WorkflowName: raw.Name,
		WorkflowPath: raw.Path,
		HeadSHA:      raw.HeadSHA,
		HeadBranch:   raw.HeadBranch,
		Number:       raw.Number,
		Status:       raw.Status,
		Conclusion:   raw.Conclusion,
		WebURL:       raw.HTMLURL,
	}, nil
}
```

- [ ] **Step 4: Implement `WorkflowFile` and refactor `WorkflowInputs` onto it**

In `internal/gh/client_inputs.go`, replace the whole `WorkflowInputs` function with:

```go
// WorkflowFile fetches a file from the repo at ref (empty ref = default
// branch) and returns its decoded bytes.
func (c *Client) WorkflowFile(repo RepoRef, path, ref string) ([]byte, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/contents/%s", repo.Owner, repo.Name, path)
	if ref != "" {
		endpoint += "?ref=" + url.QueryEscape(ref)
	}
	out, err := c.run.Exec("api", endpoint, "--jq", ".content")
	if err != nil {
		return nil, err
	}
	// Strip both \n and \r: GitHub line-wraps base64 with \n, but some proxies
	// inject \r\n, which would otherwise make the decode fail on a stray \r.
	cleaned := strings.NewReplacer("\n", "", "\r", "").Replace(strings.TrimSpace(string(out)))
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("decoding workflow content: %w", err)
	}
	return decoded, nil
}

// WorkflowInputs fetches the workflow file and parses its workflow_dispatch inputs in order.
func (c *Client) WorkflowInputs(repo RepoRef, path string) ([]Input, error) {
	decoded, err := c.WorkflowFile(repo, path, "")
	if err != nil {
		return nil, err
	}
	return parseDispatchInputs(decoded)
}
```

Add `"net/url"` to the import block of `client_inputs.go`. `encoding/base64`, `fmt` and `strings` are already imported; `errors` stays (used by `ErrNoDispatch`).

- [ ] **Step 5: Run the package tests**

Run: `go test ./internal/gh/ -v`
Expected: PASS, including the pre-existing `TestWorkflowInputsParses` (which asserts the argv is `api repos/o/r/contents/.github/workflows/deploy.yml` — unchanged, because `WorkflowInputs` passes an empty ref).

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: all packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add internal/gh/client_runmeta.go internal/gh/client_runmeta_test.go \
        internal/gh/client_inputs.go internal/gh/client_inputs_test.go internal/gh/types.go
git commit -m "feat(gh): add RunMeta and WorkflowFile, refactor WorkflowInputs onto it

RunMeta goes through gh api because the workflow file path is not among
the fields gh run view --json can return.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: chat — `Context`, `Session`, `Prepare`

**Files:**
- Create: `internal/chat/types.go`
- Create: `internal/chat/prepare.go`
- Create: `internal/chat/prepare_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (pure stdlib).
- Produces:
  - `type Context struct { Repo string; RunID int64; RunNumber int; Workflow, WorkflowPath, Branch, HeadSHA, Status, Conclusion string; FailedSteps []string; WebURL string }`
  - `func (c Context) Failed() bool`
  - `type Session struct { Context; Dir, LogFile, WorkflowFile, CloneDir string }`
  - `var ErrNoContext error`
  - `func Prepare(baseDir string, ctx Context, log, workflowYAML []byte) (Session, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/chat/prepare_test.go`:

```go
package chat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sampleContext() Context {
	return Context{
		Repo:         "acme/widgets",
		RunID:        4211,
		RunNumber:    17,
		Workflow:     "Build & test",
		WorkflowPath: ".github/workflows/ci.yaml",
		Branch:       "feature/x",
		HeadSHA:      "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0",
		Status:       "completed",
		Conclusion:   "failure",
		FailedSteps:  []string{"build / npm ci"},
		WebURL:       "https://github.com/acme/widgets/actions/runs/4211",
	}
}

func TestPrepareWritesBothFiles(t *testing.T) {
	base := t.TempDir()
	s, err := Prepare(base, sampleContext(), []byte("log body"), []byte("name: CI\n"))
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(base, "acme", "widgets", "4211")
	if s.Dir != wantDir {
		t.Errorf("Dir = %q, want %q", s.Dir, wantDir)
	}
	if s.LogFile != filepath.Join(wantDir, "log.txt") {
		t.Errorf("LogFile = %q", s.LogFile)
	}
	if s.WorkflowFile != filepath.Join(wantDir, "workflow.yml") {
		t.Errorf("WorkflowFile = %q", s.WorkflowFile)
	}
	if b, err := os.ReadFile(s.LogFile); err != nil || string(b) != "log body" {
		t.Errorf("log content = %q, err %v", b, err)
	}
	if b, err := os.ReadFile(s.WorkflowFile); err != nil || string(b) != "name: CI\n" {
		t.Errorf("workflow content = %q, err %v", b, err)
	}
	// The Context travels with the Session.
	if s.Repo != "acme/widgets" || s.RunID != 4211 {
		t.Errorf("context not carried: %+v", s.Context)
	}
}

func TestPrepareSkipsEmptyLog(t *testing.T) {
	base := t.TempDir()
	s, err := Prepare(base, sampleContext(), nil, []byte("name: CI\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.LogFile != "" {
		t.Errorf("LogFile = %q, want empty", s.LogFile)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "log.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("log.txt must not be created when there is no log")
	}
	if s.WorkflowFile == "" {
		t.Error("the workflow file should still have been written")
	}
}

func TestPrepareSkipsEmptyWorkflow(t *testing.T) {
	base := t.TempDir()
	s, err := Prepare(base, sampleContext(), []byte("log body"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.WorkflowFile != "" {
		t.Errorf("WorkflowFile = %q, want empty", s.WorkflowFile)
	}
	if s.LogFile == "" {
		t.Error("the log should still have been written")
	}
}

func TestPrepareRejectsEmptyContext(t *testing.T) {
	base := t.TempDir()
	_, err := Prepare(base, sampleContext(), nil, nil)
	if !errors.Is(err, ErrNoContext) {
		t.Fatalf("err = %v, want ErrNoContext", err)
	}
	// Nothing was created on disk.
	if _, err := os.Stat(filepath.Join(base, "acme")); !errors.Is(err, os.ErrNotExist) {
		t.Error("Prepare must not create directories when there is no context")
	}
}

func TestPrepareClearsStaleFiles(t *testing.T) {
	base := t.TempDir()
	if _, err := Prepare(base, sampleContext(), []byte("old log"), []byte("old wf")); err != nil {
		t.Fatal(err)
	}
	// Second run: the workflow could not be fetched this time.
	s, err := Prepare(base, sampleContext(), []byte("new log"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "workflow.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a stale workflow.yml from a previous run must be cleared")
	}
	if b, _ := os.ReadFile(s.LogFile); string(b) != "new log" {
		t.Errorf("log = %q, want the fresh one", b)
	}
}

func TestContextFailed(t *testing.T) {
	for _, tc := range []struct {
		conclusion string
		want       bool
	}{
		{"failure", true},
		{"timed_out", true},
		{"success", false},
		{"cancelled", false},
		{"", false},
	} {
		if got := (Context{Conclusion: tc.conclusion}).Failed(); got != tc.want {
			t.Errorf("Failed(%q) = %v, want %v", tc.conclusion, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/chat/ -v`
Expected: FAIL — the package does not exist yet (`no Go files` / `undefined: Prepare`).

- [ ] **Step 3: Implement the types**

Create `internal/chat/types.go`:

```go
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
```

- [ ] **Step 4: Implement `Prepare`**

Create `internal/chat/prepare.go`:

```go
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
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/chat/ -v`
Expected: PASS — `TestPrepareWritesBothFiles`, `TestPrepareSkipsEmptyLog`, `TestPrepareSkipsEmptyWorkflow`, `TestPrepareRejectsEmptyContext`, `TestPrepareClearsStaleFiles`, `TestContextFailed`.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: all packages `ok`.

- [ ] **Step 7: Commit**

```bash
git add internal/chat/types.go internal/chat/prepare.go internal/chat/prepare_test.go
git commit -m "feat(chat): materialise a run's log and workflow on disk

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: chat — the initial prompt

**Files:**
- Create: `internal/chat/prompt.go`
- Create: `internal/chat/prompt_test.go`

**Interfaces:**
- Consumes: `Session`, `Context.Failed()` from Task 3.
- Produces: `func Prompt(s Session) string`

- [ ] **Step 1: Write the failing test**

Create `internal/chat/prompt_test.go`:

```go
package chat

import (
	"strings"
	"testing"
)

// failedSession is the nominal case: failed run, both files, clone found.
func failedSession() Session {
	return Session{
		Context:      sampleContext(),
		Dir:          "/cache/acme/widgets/4211",
		LogFile:      "/cache/acme/widgets/4211/log.txt",
		WorkflowFile: "/cache/acme/widgets/4211/workflow.yml",
		CloneDir:     "/home/me/dev/widgets",
	}
}

func TestPromptFailedRun(t *testing.T) {
	p := Prompt(failedSession())
	for _, want := range []string{
		"GitHub Actions run #17 of acme/widgets — failure",
		"Workflow: Build & test (.github/workflows/ci.yaml)",
		"Branch: feature/x · commit df53ccd · https://github.com/acme/widgets/actions/runs/4211",
		"Failed steps: build / npm ci",
		"/cache/acme/widgets/4211/log.txt",
		"/cache/acme/widgets/4211/workflow.yml",
		"as it was at df53ccd",
		"You are in a local clone of this repository",
		"gh run view 4211 --repo acme/widgets --log",
		"Analyse the root cause of this failure, then propose a fix.",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}

func TestPromptSuccessfulRunAsksForASummary(t *testing.T) {
	s := failedSession()
	s.Conclusion = "success"
	s.FailedSteps = nil
	p := Prompt(s)
	if !strings.Contains(p, "— success") {
		t.Errorf("state line wrong:\n%s", p)
	}
	if strings.Contains(p, "Failed steps:") {
		t.Errorf("no failed steps means no such line:\n%s", p)
	}
	if !strings.Contains(p, "Summarise what this run did.") {
		t.Errorf("expected the summary question:\n%s", p)
	}
	if strings.Contains(p, "root cause") {
		t.Errorf("a successful run must not ask for a root cause:\n%s", p)
	}
}

func TestPromptWithoutCloneOmitsTheCloneLine(t *testing.T) {
	s := failedSession()
	s.CloneDir = ""
	if strings.Contains(Prompt(s), "local clone") {
		t.Errorf("no clone means no clone line:\n%s", Prompt(s))
	}
}

func TestPromptWithoutLogSaysSo(t *testing.T) {
	s := failedSession()
	s.LogFile = ""
	p := Prompt(s)
	if strings.Contains(p, "log.txt") {
		t.Errorf("must not reference a log that was not written:\n%s", p)
	}
	if !strings.Contains(p, "no log could be fetched") {
		t.Errorf("expected the missing-log notice:\n%s", p)
	}
}

func TestPromptWithoutWorkflowSaysSo(t *testing.T) {
	s := failedSession()
	s.WorkflowFile = ""
	p := Prompt(s)
	if strings.Contains(p, "workflow.yml") {
		t.Errorf("must not reference a workflow file that was not written:\n%s", p)
	}
	if !strings.Contains(p, "workflow file could not be fetched") {
		t.Errorf("expected the missing-workflow notice:\n%s", p)
	}
}

func TestPromptFallsBackToStatusWhenNotConcluded(t *testing.T) {
	s := failedSession()
	s.Status, s.Conclusion = "in_progress", ""
	if !strings.Contains(Prompt(s), "— in_progress") {
		t.Errorf("expected the status as the state:\n%s", Prompt(s))
	}
}

func TestPromptKeepsAShortSHAAsIs(t *testing.T) {
	s := failedSession()
	s.HeadSHA = "abc"
	if !strings.Contains(Prompt(s), "commit abc") {
		t.Errorf("a short sha must be printed as-is:\n%s", Prompt(s))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/chat/ -run Prompt -v`
Expected: FAIL — `undefined: Prompt`.

- [ ] **Step 3: Implement**

Create `internal/chat/prompt.go`:

```go
package chat

import (
	"fmt"
	"strings"
)

// shortSHALen is how much of a commit sha the prompt shows.
const shortSHALen = 7

// Prompt renders the first message handed to claude: what the run is, where
// its context files are, and the question to start on. It is deliberately
// terse — Claude reads the files itself rather than receiving them inline.
func Prompt(s Session) string {
	var b strings.Builder

	state := s.Conclusion
	if state == "" {
		state = s.Status
	}
	fmt.Fprintf(&b, "GitHub Actions run #%d of %s — %s\n", s.RunNumber, s.Repo, state)

	if s.Workflow != "" {
		if s.WorkflowPath != "" {
			fmt.Fprintf(&b, "Workflow: %s (%s)\n", s.Workflow, s.WorkflowPath)
		} else {
			fmt.Fprintf(&b, "Workflow: %s\n", s.Workflow)
		}
	}
	if s.Branch != "" {
		fmt.Fprintf(&b, "Branch: %s", s.Branch)
		if s.HeadSHA != "" {
			fmt.Fprintf(&b, " · commit %s", shortSHA(s.HeadSHA))
		}
		if s.WebURL != "" {
			fmt.Fprintf(&b, " · %s", s.WebURL)
		}
		b.WriteString("\n")
	}
	if len(s.FailedSteps) > 0 {
		fmt.Fprintf(&b, "Failed steps: %s\n", strings.Join(s.FailedSteps, "; "))
	}

	b.WriteString("\nContext files (read them before answering):\n")
	if s.LogFile != "" {
		fmt.Fprintf(&b, "  %s — the run log (may cover only the failed jobs)\n", s.LogFile)
	} else {
		b.WriteString("  no log could be fetched — the run may still be in progress\n")
	}
	if s.WorkflowFile != "" {
		if s.HeadSHA != "" {
			fmt.Fprintf(&b, "  %s — the workflow file as it was at %s\n", s.WorkflowFile, shortSHA(s.HeadSHA))
		} else {
			fmt.Fprintf(&b, "  %s — the workflow file\n", s.WorkflowFile)
		}
	} else {
		b.WriteString("  the workflow file could not be fetched\n")
	}

	b.WriteString("\n")
	if s.CloneDir != "" {
		b.WriteString("You are in a local clone of this repository (it may be on another commit).\n")
	}
	fmt.Fprintf(&b, "For the full log of every job: gh run view %d --repo %s --log\n", s.RunID, s.Repo)

	fmt.Fprintf(&b, "\n%s\n", question(s))
	return b.String()
}

// question is the task the first message ends on.
func question(s Session) string {
	if s.Failed() {
		return "Analyse the root cause of this failure, then propose a fix."
	}
	return "Summarise what this run did."
}

func shortSHA(sha string) string {
	if len(sha) > shortSHALen {
		return sha[:shortSHALen]
	}
	return sha
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/chat/ -v`
Expected: PASS (Task 3's tests plus the seven new prompt tests).

- [ ] **Step 5: Eyeball the rendered prompt once**

Run:
```bash
cat > /tmp/prompt_demo_test.go <<'EOF'
package chat

import "testing"

func TestDemoPrintPrompt(t *testing.T) { t.Log("\n" + Prompt(failedSession())) }
EOF
cp /tmp/prompt_demo_test.go internal/chat/zz_demo_test.go
go test ./internal/chat/ -run TestDemoPrintPrompt -v
rm internal/chat/zz_demo_test.go
```
Expected: the full prompt printed. Read it — it is what Claude will receive. Fix any awkward wording before committing, then delete the throwaway file (the `rm` above does it).

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: all packages `ok`. Confirm `internal/chat/zz_demo_test.go` is gone: `git status --short` must not list it.

- [ ] **Step 7: Commit**

```bash
git add internal/chat/prompt.go internal/chat/prompt_test.go
git commit -m "feat(chat): render the initial prompt handed to claude

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: chat — locate the clone, build the command

**Files:**
- Create: `internal/chat/launch.go`
- Create: `internal/chat/launch_test.go`

**Interfaces:**
- Consumes: `Session` (Task 3), `Prompt` (Task 4), `config.ExpandHome` (Task 1).
- Produces:
  - `func FindClone(roots []string, owner, name string) (string, bool)`
  - `func Command(claudeCmd string, s Session) *exec.Cmd`

- [ ] **Step 1: Write the failing test**

Create `internal/chat/launch_test.go`:

```go
package chat

import (
	"os"
	"path/filepath"
	"testing"
)

// mkClone creates dir plus a .git directory inside it.
func mkClone(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFindCloneByName(t *testing.T) {
	root := t.TempDir()
	want := mkClone(t, root, "widgets")
	got, ok := FindClone([]string{root}, "acme", "widgets")
	if !ok || got != want {
		t.Fatalf("FindClone = %q, %v; want %q, true", got, ok, want)
	}
}

func TestFindCloneByOwnerAndName(t *testing.T) {
	root := t.TempDir()
	want := mkClone(t, root, "acme", "widgets")
	got, ok := FindClone([]string{root}, "acme", "widgets")
	if !ok || got != want {
		t.Fatalf("FindClone = %q, %v; want %q, true", got, ok, want)
	}
}

func TestFindClonePrefersBareNameOverOwnerPath(t *testing.T) {
	root := t.TempDir()
	bare := mkClone(t, root, "widgets")
	mkClone(t, root, "acme", "widgets")
	got, _ := FindClone([]string{root}, "acme", "widgets")
	if got != bare {
		t.Fatalf("FindClone = %q, want the bare-name candidate %q", got, bare)
	}
}

func TestFindCloneSkipsDirectoryWithoutGit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := FindClone([]string{root}, "acme", "widgets"); ok {
		t.Fatalf("a directory without .git must not match, got %q", got)
	}
}

func TestFindCloneAcceptsGitFileForWorktrees(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "widgets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A linked worktree has a .git *file*, not a directory.
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := FindClone([]string{root}, "acme", "widgets"); !ok || got != dir {
		t.Fatalf("FindClone = %q, %v; want %q, true", got, ok, dir)
	}
}

func TestFindCloneScansRootsInOrder(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	want := mkClone(t, second, "widgets")
	got, ok := FindClone([]string{first, second}, "acme", "widgets")
	if !ok || got != want {
		t.Fatalf("FindClone = %q, %v; want %q, true", got, ok, want)
	}
}

func TestFindCloneNoRootsNoMatch(t *testing.T) {
	if _, ok := FindClone(nil, "acme", "widgets"); ok {
		t.Error("no roots must mean no match")
	}
	if _, ok := FindClone([]string{"", "/does/not/exist"}, "acme", "widgets"); ok {
		t.Error("empty and missing roots must mean no match")
	}
}

func TestCommandRunsInTheCloneAndAllowsTheContextDir(t *testing.T) {
	s := failedSession()
	cmd := Command("claude", s)

	if cmd.Dir != "/home/me/dev/widgets" {
		t.Errorf("Dir = %q, want the clone", cmd.Dir)
	}
	// cmd.Args[0] is the command name.
	if len(cmd.Args) != 4 {
		t.Fatalf("args = %v", cmd.Args)
	}
	if cmd.Args[1] != "--add-dir" || cmd.Args[2] != s.Dir {
		t.Errorf("args = %v, want --add-dir %s", cmd.Args, s.Dir)
	}
	if cmd.Args[3] != Prompt(s) {
		t.Errorf("the prompt must be the last argument, got %q", cmd.Args[3])
	}
	if cmd.Path == "" {
		t.Error("Path must be resolved (or left as the bare name when not on PATH)")
	}
}

func TestCommandFallsBackToTheContextDir(t *testing.T) {
	s := failedSession()
	s.CloneDir = ""
	cmd := Command("claude", s)
	if cmd.Dir != s.Dir {
		t.Errorf("Dir = %q, want the context dir %q", cmd.Dir, s.Dir)
	}
}

func TestCommandHonorsACustomBinary(t *testing.T) {
	cmd := Command("my-claude", failedSession())
	if cmd.Args[0] != "my-claude" {
		t.Errorf("Args[0] = %q", cmd.Args[0])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/chat/ -run 'FindClone|Command' -v`
Expected: FAIL — `undefined: FindClone`, `undefined: Command`.

- [ ] **Step 3: Implement**

Create `internal/chat/launch.go`:

```go
package chat

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stephaneHerraiz/ghrun/internal/config"
)

// FindClone locates the repository's local clone under one of roots, so the
// chat can start where the code actually is. It tries "<root>/<name>" then
// "<root>/<owner>/<name>", and only accepts a candidate that holds a .git
// entry — a directory for a normal clone, a file for a linked worktree.
func FindClone(roots []string, owner, name string) (string, bool) {
	for _, root := range roots {
		r := config.ExpandHome(root)
		if r == "" {
			continue
		}
		for _, cand := range []string{
			filepath.Join(r, name),
			filepath.Join(r, owner, name),
		} {
			if _, err := os.Stat(filepath.Join(cand, ".git")); err == nil {
				return cand, true
			}
		}
	}
	return "", false
}

// Command builds the interactive claude invocation. The context files live
// outside the clone, so --add-dir is what lets Claude Code read them.
func Command(claudeCmd string, s Session) *exec.Cmd {
	cmd := exec.Command(claudeCmd, "--add-dir", s.Dir, Prompt(s))
	cmd.Dir = s.Dir
	if s.CloneDir != "" {
		cmd.Dir = s.CloneDir
	}
	return cmd
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/chat/ -v`
Expected: PASS (all of Tasks 3, 4 and 5).

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/chat/launch.go internal/chat/launch_test.go
git commit -m "feat(chat): locate the local clone and build the claude command

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: UI — messages and `prepareChatCmd`

**Files:**
- Modify: `internal/ui/client.go` (GHClient gains two methods)
- Modify: `internal/ui/messages.go`
- Create: `internal/ui/chat_test.go`

**Interfaces:**
- Consumes: `gh.RunMeta`, `Client.RunMeta`, `Client.WorkflowFile` (Task 2); `chat.Context`, `chat.Prepare`, `chat.FindClone`, `chat.Command` (Tasks 3–5); `config.ChatConfig` (Task 1); the existing `failedSteps(gh.RunDetail) []string` from `explain.go`.
- Produces:
  - `type chatFetcher interface { RunMeta(...); WorkflowFile(...); RunLogs(...) }`
  - `type chatRequestMsg struct { repo gh.RepoRef; id int64; detail gh.RunDetail }`
  - `type chatReadyMsg struct { cmd *exec.Cmd }`
  - `type chatDoneMsg struct { err error }`
  - `func prepareChatCmd(f chatFetcher, cfg config.ChatConfig, cacheDir string, repo gh.RepoRef, id int64, detail gh.RunDetail) tea.Cmd`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/chat_test.go`:

```go
package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// fakeChatFetcher is the narrow client prepareChatCmd needs.
type fakeChatFetcher struct {
	meta    gh.RunMeta
	metaErr error
	wf      []byte
	wfErr   error
	// logs is keyed by failedOnly.
	logs    map[bool]string
	logErr  error
	wfCalls [][2]string // path, ref
}

func (f *fakeChatFetcher) RunMeta(gh.RepoRef, int64) (gh.RunMeta, error) {
	return f.meta, f.metaErr
}

func (f *fakeChatFetcher) WorkflowFile(_ gh.RepoRef, path, ref string) ([]byte, error) {
	f.wfCalls = append(f.wfCalls, [2]string{path, ref})
	return f.wf, f.wfErr
}

func (f *fakeChatFetcher) RunLogs(_ gh.RepoRef, _ int64, failedOnly bool) (string, error) {
	if f.logErr != nil {
		return "", f.logErr
	}
	return f.logs[failedOnly], nil
}

func okFetcher() *fakeChatFetcher {
	return &fakeChatFetcher{
		meta: gh.RunMeta{
			WorkflowName: "Build & test",
			WorkflowPath: ".github/workflows/ci.yaml",
			HeadSHA:      "df53ccd81d93cdb8c4555f83fee5c7eacf95d9e0",
			HeadBranch:   "feature/x",
			Number:       17,
			Status:       "completed",
			Conclusion:   "failure",
			WebURL:       "https://github.com/acme/widgets/actions/runs/4211",
		},
		wf:   []byte("name: CI\n"),
		logs: map[bool]string{true: "npm ERR! boom"},
	}
}

func failedRunDetail() gh.RunDetail {
	return gh.RunDetail{
		Run: gh.Run{ID: 4211, Status: "completed", Conclusion: "failure"},
		Jobs: []gh.Job{{
			Name: "build", Conclusion: "failure",
			Steps: []gh.Step{
				{Name: "checkout", Conclusion: "success"},
				{Name: "npm ci", Conclusion: "failure"},
			},
		}},
	}
}

// chatCfg points claudeCmd at a binary guaranteed to be on PATH so the
// LookPath guard passes without a real claude install.
func chatCfg() config.ChatConfig {
	return config.ChatConfig{ClaudeCmd: "go", CloneRoots: nil}
}

func repoRef() gh.RepoRef { return gh.RepoRef{Owner: "acme", Name: "widgets"} }

func TestPrepareChatBuildsTheCommand(t *testing.T) {
	base := t.TempDir()
	f := okFetcher()
	msg := prepareChatCmd(f, chatCfg(), base, repoRef(), 4211, failedRunDetail())()

	ready, ok := msg.(chatReadyMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	prompt := ready.cmd.Args[len(ready.cmd.Args)-1]
	for _, want := range []string{
		"run #17 of acme/widgets — failure",
		"Build & test (.github/workflows/ci.yaml)",
		"Failed steps: build / npm ci",
		"Analyse the root cause",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	// The workflow was fetched at the run's commit, not at HEAD.
	if len(f.wfCalls) != 1 || f.wfCalls[0][1] != f.meta.HeadSHA {
		t.Errorf("workflow calls = %v", f.wfCalls)
	}
	// Both files landed on disk.
	dir := filepath.Join(base, "acme", "widgets", "4211")
	if _, err := os.Stat(filepath.Join(dir, "log.txt")); err != nil {
		t.Errorf("log.txt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "workflow.yml")); err != nil {
		t.Errorf("workflow.yml: %v", err)
	}
}

func TestPrepareChatUsesFullLogForASuccessfulRun(t *testing.T) {
	f := okFetcher()
	f.meta.Conclusion = "success"
	f.logs = map[bool]string{false: "everything fine"}
	detail := failedRunDetail()
	detail.Conclusion = "success"
	detail.Jobs[0].Steps[1].Conclusion = "success"

	msg := prepareChatCmd(f, chatCfg(), t.TempDir(), repoRef(), 4211, detail)()
	ready, ok := msg.(chatReadyMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	prompt := ready.cmd.Args[len(ready.cmd.Args)-1]
	if !strings.Contains(prompt, "Summarise what this run did.") {
		t.Errorf("expected the summary question:\n%s", prompt)
	}
}

func TestPrepareChatFallsBackToTheFullLogWhenFailedOnlyIsEmpty(t *testing.T) {
	base := t.TempDir()
	f := okFetcher()
	f.logs = map[bool]string{true: "   ", false: "the whole log"}
	msg := prepareChatCmd(f, chatCfg(), base, repoRef(), 4211, failedRunDetail())()
	if _, ok := msg.(chatReadyMsg); !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	b, err := os.ReadFile(filepath.Join(base, "acme", "widgets", "4211", "log.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "the whole log" {
		t.Errorf("log = %q, want the full-log fallback", b)
	}
}

func TestPrepareChatSurvivesMissingMeta(t *testing.T) {
	f := okFetcher()
	f.metaErr = errors.New("api down")
	msg := prepareChatCmd(f, chatCfg(), t.TempDir(), repoRef(), 4211, failedRunDetail())()
	ready, ok := msg.(chatReadyMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	prompt := ready.cmd.Args[len(ready.cmd.Args)-1]
	if !strings.Contains(prompt, "workflow file could not be fetched") {
		t.Errorf("expected the missing-workflow notice:\n%s", prompt)
	}
	// A web URL is still built from the repo and id.
	if !strings.Contains(prompt, "https://github.com/acme/widgets/actions/runs/4211") {
		t.Errorf("expected a fallback web url:\n%s", prompt)
	}
	if len(f.wfCalls) != 0 {
		t.Errorf("no workflow path means no contents call, got %v", f.wfCalls)
	}
}

func TestPrepareChatErrorsWhenNothingCanBeFetched(t *testing.T) {
	f := &fakeChatFetcher{metaErr: errors.New("nope"), logErr: errors.New("nope")}
	msg := prepareChatCmd(f, chatCfg(), t.TempDir(), repoRef(), 4211, failedRunDetail())()
	em, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	if !strings.Contains(em.err.Error(), "chat") {
		t.Errorf("err = %v", em.err)
	}
}

func TestPrepareChatErrorsWhenClaudeIsNotInstalled(t *testing.T) {
	cfg := config.ChatConfig{ClaudeCmd: "definitely-not-a-real-binary-xyz"}
	msg := prepareChatCmd(okFetcher(), cfg, t.TempDir(), repoRef(), 4211, failedRunDetail())()
	em, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("msg = %T (%v)", msg, msg)
	}
	if !strings.Contains(em.err.Error(), "definitely-not-a-real-binary-xyz") {
		t.Errorf("err = %v", em.err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ui/ -run PrepareChat -v`
Expected: FAIL — `undefined: prepareChatCmd`, `undefined: chatReadyMsg`.

- [ ] **Step 3: Extend the `GHClient` interface**

In `internal/ui/client.go`, add two methods to `GHClient`, after `RunLogs`:

```go
	RunMeta(repo gh.RepoRef, id int64) (gh.RunMeta, error)
	WorkflowFile(repo gh.RepoRef, path, ref string) ([]byte, error)
```

- [ ] **Step 4: Implement the messages and the command**

Append to `internal/ui/messages.go`:

```go
// --- chat feature ---

// chatFetcher is the subset of GHClient prepareChatCmd needs. Keeping it
// narrow means the test fake is three methods rather than the whole client.
type chatFetcher interface {
	RunMeta(repo gh.RepoRef, id int64) (gh.RunMeta, error)
	WorkflowFile(repo gh.RepoRef, path, ref string) ([]byte, error)
	RunLogs(repo gh.RepoRef, id int64, failedOnly bool) (string, error)
}

// chatRequestMsg asks the App (which owns the config and the cache dir) to
// prepare a chat about this run.
type chatRequestMsg struct {
	repo   gh.RepoRef
	id     int64
	detail gh.RunDetail
}

// chatReadyMsg carries the claude invocation the App hands to tea.ExecProcess.
type chatReadyMsg struct{ cmd *exec.Cmd }

// chatDoneMsg reports that claude exited and the terminal is ours again.
type chatDoneMsg struct{ err error }

// prepareChatCmd gathers the run's context, writes it under cacheDir and
// builds the claude invocation. Every fetch is best-effort: a missing piece
// is described in the prompt rather than aborting, and only a completely
// empty context is an error.
func prepareChatCmd(f chatFetcher, cfg config.ChatConfig, cacheDir string,
	repo gh.RepoRef, id int64, detail gh.RunDetail) tea.Cmd {
	return func() tea.Msg {
		if _, err := exec.LookPath(cfg.ClaudeCmd); err != nil {
			return errMsg{err: fmt.Errorf("chat: %q not found in PATH", cfg.ClaudeCmd)}
		}

		cc := chat.Context{
			Repo:        repo.String(),
			RunID:       id,
			Status:      detail.Status,
			Conclusion:  detail.Conclusion,
			FailedSteps: failedSteps(detail),
			WebURL:      fmt.Sprintf("https://github.com/%s/actions/runs/%d", repo.String(), id),
		}
		meta, metaErr := f.RunMeta(repo, id)
		if metaErr == nil {
			cc.Workflow = meta.WorkflowName
			cc.WorkflowPath = meta.WorkflowPath
			cc.Branch = meta.HeadBranch
			cc.HeadSHA = meta.HeadSHA
			cc.RunNumber = meta.Number
			if meta.Conclusion != "" {
				cc.Conclusion = meta.Conclusion
			}
			if meta.Status != "" {
				cc.Status = meta.Status
			}
			if meta.WebURL != "" {
				cc.WebURL = meta.WebURL
			}
		}

		// The failed-job log is the useful one on a failure; fall back to the
		// full log when it is empty (e.g. a cancelled job leaves it blank).
		logText, _ := f.RunLogs(repo, id, cc.Failed())
		if cc.Failed() && strings.TrimSpace(logText) == "" {
			logText, _ = f.RunLogs(repo, id, false)
		}

		var wf []byte
		if cc.WorkflowPath != "" {
			wf, _ = f.WorkflowFile(repo, cc.WorkflowPath, cc.HeadSHA)
		}

		s, err := chat.Prepare(cacheDir, cc, []byte(logText), wf)
		if err != nil {
			return errMsg{err: fmt.Errorf("chat: %w", err)}
		}
		s.CloneDir, _ = chat.FindClone(cfg.CloneRoots, repo.Owner, repo.Name)
		return chatReadyMsg{cmd: chat.Command(cfg.ClaudeCmd, s)}
	}
}
```

Add `"os/exec"` and `"github.com/stephaneHerraiz/ghrun/internal/chat"` to the imports of `messages.go` (`fmt`, `strings`, `config` and `gh` are already there).

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/ui/ -run PrepareChat -v`
Expected: PASS — six tests.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: all packages `ok`. (Adding methods to `GHClient` compiles because the only production implementation is `*gh.Client`, which gained them in Task 2, and the UI tests pass `nil`.)

- [ ] **Step 7: Commit**

```bash
git add internal/ui/client.go internal/ui/messages.go internal/ui/chat_test.go
git commit -m "feat(ui): prepare a run's chat context and claude invocation

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 7: UI — the `c` key and the App suspend/exec wiring

**Files:**
- Modify: `internal/ui/rundetail.go`
- Modify: `internal/ui/rundetail_test.go`
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/app_test.go`

**Interfaces:**
- Consumes: `chatRequestMsg`, `chatReadyMsg`, `chatDoneMsg`, `prepareChatCmd` (Task 6); `config.ChatConfig.IsEnabled()` (Task 1).
- Produces: `func (a App) WithChatCacheDir(dir string) App`

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/rundetail_test.go`:

```go
func TestRunDetailChatKeyEmitsMsg(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.chatAvailable = true
	rd.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 5, Status: "completed", Conclusion: "success"}}})
	_, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("'c' must emit chatRequestMsg on any loaded run")
	}
	msg, ok := cmd().(chatRequestMsg)
	if !ok {
		t.Fatalf("msg = %T", cmd())
	}
	if msg.id != 5 || msg.repo.Name != "r" {
		t.Errorf("msg = %+v", msg)
	}
}

func TestRunDetailChatKeyInertWhenUnavailable(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.chatAvailable = false
	rd.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 5, Status: "completed", Conclusion: "success"}}})
	if _, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}); cmd != nil {
		t.Fatal("'c' must do nothing when chat is unavailable")
	}
}

func TestRunDetailChatKeyInertBeforeLoad(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.chatAvailable = true
	if _, cmd := rd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}); cmd != nil {
		t.Fatal("'c' must wait for the run detail to load")
	}
}

func TestRunDetailShowsChatHint(t *testing.T) {
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	rd.chatAvailable = true
	s, _ := rd.Update(runDetailLoadedMsg{detail: gh.RunDetail{Run: gh.Run{ID: 5, Status: "completed", Conclusion: "success"}}})
	if !strings.Contains(s.View(), "c chat") {
		t.Errorf("view missing the chat hint:\n%s", s.View())
	}
}
```

Append to `internal/ui/app_test.go`:

```go
func TestAppStampsChatAvailabilityOnPush(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithChatCacheDir("/cache")
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	m, _ := a.Update(pushMsg{screen: rd})
	if !m.(App).top().(*rundetail).chatAvailable {
		t.Error("chat should be available: enabled by default and a cache dir is set")
	}
}

func TestAppChatUnavailableWithoutCacheDir(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"})
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	m, _ := a.Update(pushMsg{screen: rd})
	if m.(App).top().(*rundetail).chatAvailable {
		t.Error("no cache dir means no chat")
	}
}

func TestAppChatUnavailableWhenDisabled(t *testing.T) {
	off := false
	cfg := config.Config{DefaultOrg: "acme", Chat: config.ChatConfig{Enabled: &off}}
	a := NewApp(nil, cfg).WithChatCacheDir("/cache")
	rd, _ := newRunDetail(nil, gh.RepoRef{Owner: "o", Name: "r"}, 5)
	m, _ := a.Update(pushMsg{screen: rd})
	if m.(App).top().(*rundetail).chatAvailable {
		t.Error("chat.enabled: false must hide the feature")
	}
}

func TestAppSuspendsTickerWhileChatting(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithChatCacheDir("/cache")

	m, cmd := a.Update(chatReadyMsg{cmd: exec.Command("true")})
	if cmd == nil {
		t.Fatal("chatReadyMsg must return the exec command")
	}
	if !m.(App).suspended {
		t.Fatal("the app must be marked suspended")
	}
	// A tick arriving while suspended is dropped, ticker chain included.
	if _, tickCmd := m.(App).Update(tickMsg(time.Now())); tickCmd != nil {
		t.Error("ticks must be dropped while suspended, with no rescheduling")
	}
}

func TestAppResumesAfterChat(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithChatCacheDir("/cache")
	m, _ := a.Update(chatReadyMsg{cmd: exec.Command("true")})

	resumed, cmd := m.(App).Update(chatDoneMsg{})
	if resumed.(App).suspended {
		t.Error("chatDoneMsg must clear the suspended flag")
	}
	if cmd == nil {
		t.Fatal("chatDoneMsg must restart the ticker")
	}
	// A tick is honoured again.
	if _, tickCmd := resumed.(App).Update(tickMsg(time.Now())); tickCmd == nil {
		t.Error("the ticker must run again after the chat")
	}
}

func TestAppReportsChatError(t *testing.T) {
	a := NewApp(nil, config.Config{DefaultOrg: "acme"}).WithChatCacheDir("/cache")
	m, _ := a.Update(chatReadyMsg{cmd: exec.Command("true")})
	resumed, _ := m.(App).Update(chatDoneMsg{err: errorString("claude blew up")})
	if !strings.Contains(resumed.(App).View(), "claude blew up") {
		t.Errorf("the error must reach the footer:\n%s", resumed.(App).View())
	}
}
```

`app_test.go` already imports `strings`, `time` and `internal/config`; add only `"os/exec"`. `rundetail_test.go` already imports `strings` — nothing to add there.

Note on `TestAppReportsChatError`: it asserts on the rendered footer straight after `Update`, so `chatDoneMsg` must set `a.errText` **directly** rather than emitting an `errMsg` command (Step 4 does exactly that, and still schedules the usual auto-clear tick).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run 'Chat' -v`
Expected: FAIL — `rd.chatAvailable undefined`, `a.WithChatCacheDir undefined`, `a.suspended undefined`.

- [ ] **Step 3: Add the run-detail branch**

`keys.go` is **not** touched: it holds only the global uppercase navigation
keys, and `rundetail.Update` already matches its contextual keys as literals
(`"l"`, `"e"`, `"r"`…). `"c"` follows the same pattern.

In `internal/ui/rundetail.go`, add the field next to `explainAvailable`:

```go
	// chatAvailable is stamped by the App on push: chat is enabled and a
	// context cache directory could be resolved.
	chatAvailable bool
```

Add the key branch inside `Update`'s `tea.KeyMsg` switch, after the `"e"` case:

```go
		case "c":
			if d.chatAvailable && d.loaded {
				detail := d.detail
				return d, func() tea.Msg {
					return chatRequestMsg{repo: d.repo, id: d.id, detail: detail}
				}
			}
```

In `View()`, extend the hints:

```go
	if d.chatAvailable && d.loaded {
		hints += "  ·  c chat"
	}
```

- [ ] **Step 4: Wire the App**

In `internal/ui/app.go`, add two fields to `App`:

```go
	chatCacheDir string // empty: chat unavailable
	suspended    bool   // true while an external process owns the terminal
```

Add the builder next to `WithExplainService`:

```go
// WithChatCacheDir enables the "discuss this run with Claude" feature by
// giving the App somewhere to write per-run context files.
func (a App) WithChatCacheDir(dir string) App {
	a.chatCacheDir = dir
	return a
}

// chatEnabled reports whether the chat feature can be offered.
func (a App) chatEnabled() bool {
	return a.chatCacheDir != "" && a.cfg.Chat.IsEnabled()
}
```

In the `pushMsg` case, stamp the new flag alongside `explainAvailable`:

```go
		if rd, ok := m.screen.(*rundetail); ok {
			rd.explainAvailable = a.explainSvc != nil
			rd.chatAvailable = a.chatEnabled()
		}
```

Add the three new cases before `case tickMsg`:

```go
	case chatRequestMsg:
		if !a.chatEnabled() {
			return a, nil
		}
		return a, prepareChatCmd(a.client, a.cfg.Chat, a.chatCacheDir, m.repo, m.id, m.detail)
	case chatReadyMsg:
		// tea.ExecProcess hands the terminal to claude. The ticker is parked
		// meanwhile: it would otherwise keep firing gh calls for a screen
		// nobody can see, and the resumed chain would run alongside the one
		// chatDoneMsg starts.
		a.suspended = true
		return a, tea.ExecProcess(m.cmd, func(err error) tea.Msg { return chatDoneMsg{err: err} })
	case chatDoneMsg:
		a.suspended = false
		var clear tea.Cmd
		if m.err != nil {
			// Set the banner here rather than emitting an errMsg: the ticker
			// restart below must go out in the same batch, and the auto-clear
			// mirrors what the errMsg case does.
			a.errText = fmt.Errorf("chat: %w", m.err).Error()
			clear = tea.Tick(errBannerTTL, func(time.Time) tea.Msg { return clearErrMsg{} })
		}
		var reload tea.Cmd
		if r, ok := a.top().(refresher); ok {
			reload, _ = r.refresh()
		}
		return a, tea.Batch(clear, reload, tickCmd(a.tickInterval()))
```

Guard the ticker in the `tickMsg` case — insert as its first statement:

```go
	case tickMsg:
		if a.suspended {
			return a, nil // chatDoneMsg restarts the chain
		}
```

Extend the expanded help text in `footer()`:

```go
			"Runs: r rerun · f rerun-failed · x cancel · o open web · l logs · e explain · c chat · g refresh",
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS — the four run-detail tests and the six App tests, plus everything pre-existing.

- [ ] **Step 6: Run the full suite and build**

Run: `go test ./... && go build ./...`
Expected: all packages `ok`, build succeeds.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/keys.go internal/ui/rundetail.go internal/ui/rundetail_test.go \
        internal/ui/app.go internal/ui/app_test.go
git commit -m "feat(ui): open a claude chat about a run with c

The app parks its ticker for the duration so claude owns the terminal
alone and only one ticker chain survives the round trip.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 8: Bootstrap, README, smoke test

**Files:**
- Modify: `cmd/ghrun/bootstrap.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `config.ResolveChatCacheDir`, `config.ExpandHome` (Task 1); `App.WithChatCacheDir` (Task 7).
- Produces: nothing further.

- [ ] **Step 1: Wire the cache directory in bootstrap**

In `cmd/ghrun/bootstrap.go`, inside `run()`, after the explain wiring:

```go
	if dir, err := config.ResolveChatCacheDir(); err == nil {
		app = app.WithChatCacheDir(dir)
	}
```

Delete the private `expandHome` function at the bottom of the file and replace its single call site in `buildExplainService` with the shared helper:

```go
	storePath := config.ExpandHome(ec.StorePath)
```

Remove `"path/filepath"` and `"strings"` from the imports if nothing else in the file uses them (check with `go build ./...` — an unused import is a compile error, so the build tells you).

- [ ] **Step 2: Verify it builds and the suite is green**

Run: `go build ./... && go test ./...`
Expected: build succeeds, all packages `ok`.

- [ ] **Step 3: Update the README**

Three edits.

**(a)** In the *Requirements* section, after the Ollama bullet, add:

```markdown
- **Optional, for chatting about a run**: the [`claude` CLI](https://claude.com/claude-code)
  on your `PATH`. `c` on a run detail hands the terminal over to it with the
  run's log and workflow already in context.
```

**(b)** In *Keybindings → Run detail*, add a row after `e`:

```markdown
| `c` | **Chat about the run with Claude** (log + workflow as context) |
```

**(c)** In *Features*, after the "Failure explanations" bullet:

```markdown
- **Chat about a run (`c`)**: ghrun writes the run's log and the workflow file
  *as it was at the run's commit* to `~/.cache/ghrun/chat/<owner>/<name>/<run>/`,
  then hands the terminal to the interactive `claude` CLI, started inside the
  repository's local clone when one is found under `chat.cloneRoots`. Quitting
  claude drops you back exactly where you were. Works on any run, not only
  failed ones.
```

**(d)** In *Configuration*, add the block to the sample YAML after the `explain:` section:

```yaml
chat:                             # discuss a run with the claude CLI
  enabled: true
  claudeCmd: claude
  cloneRoots:                     # where to look for the repo's local clone
    - ~/dev
```

and three rows to the config table:

```markdown
| `chat.enabled` | `true` | Toggle the "discuss this run with Claude" feature. |
| `chat.claudeCmd` | `claude` | Interactive CLI launched by `c`. |
| `chat.cloneRoots` | `[~/dev]` | Roots scanned for the repo's local clone (`<root>/<name>`, then `<root>/<owner>/<name>`). |
```

**(e)** In *How ghrun talks to GitHub*, add two rows:

```markdown
| Run metadata (workflow path, head sha) | `gh api repos/{o}/{r}/actions/runs/{id}` |
| A workflow file at a given commit | `gh api repos/{o}/{r}/contents/{path}?ref={sha}` |
```

- [ ] **Step 4: Manual smoke test**

This is the only step that needs a real terminal, a real `gh` and a real `claude`.

```bash
go build -o ghrun ./cmd/ghrun
./ghrun
```

Then:
1. Enter a repo that has runs (`Enter` on the dashboard), open a **failed** run (`Enter`).
2. The footer must show `c chat`.
3. Press `c`. Within a few seconds the TUI must disappear and `claude` must start, already working on the root-cause question.
4. Check the working directory Claude reports: it should be the local clone if one exists under `~/dev`.
5. Ask a follow-up question to confirm the session is interactive.
6. Quit claude (`/exit` or Ctrl-D). You must land back on the same run detail, and the run must refresh normally (watch a live run to confirm the ticker resumed).
7. Inspect what was written:
   ```bash
   ls -la ~/.cache/ghrun/chat/*/*/*/
   ```
   Expected: `log.txt` and `workflow.yml`.
8. Repeat on a **successful** run: the question must be the summary one, and the log must be the full one.

If any step misbehaves, fix it and re-run `go test ./...` before continuing.

- [ ] **Step 5: Commit**

```bash
git add cmd/ghrun/bootstrap.go README.md
git commit -m "feat: wire the chat cache dir and document the c key

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

- [ ] **Step 6: Final verification**

```bash
go build ./... && go vet ./... && go test ./...
git status --short
```
Expected: build clean, vet silent, all tests `ok`, and the only untracked file is the `ghrun` binary (already in `.gitignore`).

---

## Spec deviations (conscious, minor)

- **`RunMeta` instead of extending `GetRun`.** The spec described a
  `RunWorkflowRef(repo, id) (path, headSHA, error)` call. `gh run view --json`
  cannot return the workflow file path, so the REST endpoint was needed anyway
  — and it hands back the run number, branch, status and web URL in the same
  request. Widening the return type to `RunMeta` avoids a second call for
  metadata the prompt wants. `GetRun` stays untouched on the hot refresh path.
- **The log's description in the prompt is generic** ("the run log (may cover
  only the failed jobs)") rather than naming `--log-failed` or `--log`. The
  failed-only fetch falls back to the full log when it comes back empty, so a
  precise label would have to be threaded through `Session` to stay accurate —
  not worth a field.
- **`config.ExpandHome` is exported** and `cmd/ghrun`'s private `expandHome` is
  deleted. The spec did not mention it; `chat.FindClone` needs the same `~/`
  handling and duplicating it in a third place was the worse option.
- **No `keyChat` constant.** The spec put `keyChat = "c"` in `keys.go`, but
  that file is documented as holding *global uppercase navigation* keys only,
  and every contextual key in `rundetail.Update` is already a literal. `"c"`
  follows the surrounding code instead of the spec here.
- **`chat.FindClone` takes `owner, name string`**, not a `gh.RepoRef`. This
  keeps `internal/chat` free of any `internal/gh` import, mirroring how
  `internal/explain` takes `Repo string`. Its only non-stdlib import is
  `internal/config`, for `ExpandHome`.
