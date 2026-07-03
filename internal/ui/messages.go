package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/config"
	"github.com/stephaneHerraiz/ghrun/internal/explain"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// Navigation messages handled by the root App.
type pushMsg struct{ screen Screen }
type gotoReposMsg struct{}
type gotoWorkflowsMsg struct{}
type gotoRunsMsg struct{}

// Org-picker messages.
type namespacesLoadedMsg struct {
	names []string
	err   error
}
type orgSelectedMsg struct{ org string }

// orgReposLoadedMsg carries the org's repositories. fromCache marks results read
// from the local cache (a fast preview) vs. a fresh gh fetch, so a slow stale
// cache read can't clobber freshly fetched data.
type orgReposLoadedMsg struct {
	repos     []gh.RepoRef
	fromCache bool
	err       error
}

// loadNamespacesCmd fetches the list of namespaces (user login + orgs) asynchronously.
func loadNamespacesCmd(c GHClient) tea.Cmd {
	return func() tea.Msg {
		names, err := c.ListNamespaces()
		return namespacesLoadedMsg{names: names, err: err}
	}
}

// loadOrgReposCmd fetches the org's repositories from gh and writes them to the
// local cache (best-effort) for instant display on the next launch.
func loadOrgReposCmd(c GHClient, org, cachePath string) tea.Cmd {
	return func() tea.Msg {
		repos, err := c.ListOrgRepos(org)
		if err != nil {
			return orgReposLoadedMsg{err: err}
		}
		if cachePath != "" {
			strs := make([]string, 0, len(repos))
			for _, r := range repos {
				strs = append(strs, r.String())
			}
			_ = config.SaveRepoCacheTo(cachePath, strs) // best-effort cache write
		}
		return orgReposLoadedMsg{repos: repos}
	}
}

// loadCachedOrgReposCmd reads the cached org repo list for instant display while
// loadOrgReposCmd refreshes from gh. Cache errors are non-fatal: an empty list.
func loadCachedOrgReposCmd(cachePath string) tea.Cmd {
	return func() tea.Msg {
		if cachePath == "" {
			return orgReposLoadedMsg{fromCache: true}
		}
		strs, err := config.LoadRepoCacheFrom(cachePath)
		if err != nil {
			return orgReposLoadedMsg{fromCache: true}
		}
		repos := make([]gh.RepoRef, 0, len(strs))
		for _, s := range strs {
			if r, err := gh.ParseRepoRef(s); err == nil {
				repos = append(repos, r)
			}
		}
		return orgReposLoadedMsg{repos: repos, fromCache: true}
	}
}

// errMsg carries a non-fatal gh error for the footer.
type errMsg struct{ err error }
type clearErrMsg struct{}

// Data messages.
type runsLoadedMsg struct {
	runs []gh.Run
	err  error
}
type runDetailLoadedMsg struct {
	detail gh.RunDetail
	err    error
}
type workflowsLoadedMsg struct {
	workflows []gh.Workflow
	err       error
}
type inputsLoadedMsg struct {
	workflowID int64
	inputs     []gh.Input
	err        error
}
type branchesLoadedMsg struct {
	branches []string
	err      error
}
type logsLoadedMsg struct {
	text string
	err  error
}
type dispatchedMsg struct {
	since time.Time
	err   error
}
type runFoundMsg struct {
	id      int64
	attempt int
	err     error
}
type actionDoneMsg struct{ err error } // rerun/cancel completed
type tickMsg time.Time

// tickCmd schedules a tick after d.
func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// --- async command helpers (one per client call) ---

func loadRunsCmd(c GHClient, repo gh.RepoRef, limit int) tea.Cmd {
	return func() tea.Msg {
		runs, err := c.ListRuns(repo, limit)
		return runsLoadedMsg{runs: runs, err: err}
	}
}

func loadRunDetailCmd(c GHClient, repo gh.RepoRef, id int64) tea.Cmd {
	return func() tea.Msg {
		d, err := c.GetRun(repo, id)
		return runDetailLoadedMsg{detail: d, err: err}
	}
}

func loadWorkflowsCmd(c GHClient, repo gh.RepoRef) tea.Cmd {
	return func() tea.Msg {
		wfs, err := c.ListWorkflows(repo)
		return workflowsLoadedMsg{workflows: wfs, err: err}
	}
}

func loadInputsCmd(c GHClient, repo gh.RepoRef, wf gh.Workflow) tea.Cmd {
	return func() tea.Msg {
		ins, err := c.WorkflowInputs(repo, wf.Path)
		return inputsLoadedMsg{workflowID: wf.ID, inputs: ins, err: err}
	}
}

func loadBranchesCmd(c GHClient, repo gh.RepoRef) tea.Cmd {
	return func() tea.Msg {
		br, err := c.ListBranches(repo)
		return branchesLoadedMsg{branches: br, err: err}
	}
}

func loadLogsCmd(c GHClient, repo gh.RepoRef, id int64, failedOnly bool) tea.Cmd {
	return func() tea.Msg {
		txt, err := c.RunLogs(repo, id, failedOnly)
		return logsLoadedMsg{text: txt, err: err}
	}
}

func rerunCmd(c GHClient, repo gh.RepoRef, id int64, failedOnly bool) tea.Cmd {
	return func() tea.Msg { return actionDoneMsg{err: c.Rerun(repo, id, failedOnly)} }
}

func cancelCmd(c GHClient, repo gh.RepoRef, id int64) tea.Cmd {
	return func() tea.Msg { return actionDoneMsg{err: c.Cancel(repo, id)} }
}

func openWebCmd(c GHClient, repo gh.RepoRef, id int64) tea.Cmd {
	return func() tea.Msg { return actionDoneMsg{err: c.OpenWeb(repo, id)} }
}

// --- explain feature ---

// explainRunMsg asks the App (which owns the explain service) to open the
// Explanation screen for a failed run.
type explainRunMsg struct {
	repo   gh.RepoRef
	id     int64
	detail gh.RunDetail
}

type explainLogLoadedMsg struct {
	id   int64
	text string
	err  error
}
type explainLocalMsg struct {
	id  int64
	res explain.LocalResult
	err error
}
type explainClaudeMsg struct {
	id  int64
	res explain.ClaudeResult
	err error
}

// explainMsgID extracts the run id an explain message belongs to, so the App
// can route it to the right explain screen.
func explainMsgID(msg tea.Msg) int64 {
	switch m := msg.(type) {
	case explainLogLoadedMsg:
		return m.id
	case explainLocalMsg:
		return m.id
	case explainClaudeMsg:
		return m.id
	}
	return 0
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
			return explainLogLoadedMsg{id: id, err: err}
		}
		if strings.TrimSpace(txt) == "" {
			return explainLogLoadedMsg{id: id, err: fmt.Errorf("no logs available for run #%d", id)}
		}
		return explainLogLoadedMsg{id: id, text: txt}
	}
}

func resolveLocalCmd(svc ExplainService, id int64, logText string) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.ResolveLocal(context.Background(), logText)
		return explainLocalMsg{id: id, res: res, err: err}
	}
}

func askClaudeCmd(svc ExplainService, id int64, req explain.ExplainRequest) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.AskClaude(context.Background(), req)
		return explainClaudeMsg{id: id, res: res, err: err}
	}
}
