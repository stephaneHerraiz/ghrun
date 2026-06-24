package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stephaneHerraiz/ghrun/internal/gh"
)

// Navigation messages handled by the root App.
type pushMsg struct{ screen Screen }
type popMsg struct{}
type gotoReposMsg struct{}
type gotoWorkflowsMsg struct{}
type gotoRunsMsg struct{}
type setRepoMsg struct{ repo gh.RepoRef }

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
