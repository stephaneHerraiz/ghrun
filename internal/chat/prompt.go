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
	} else if s.WebURL != "" {
		// No branch (the metadata call failed): the run URL is still worth
		// having on its own line rather than being dropped with it.
		fmt.Fprintf(&b, "URL: %s\n", s.WebURL)
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
