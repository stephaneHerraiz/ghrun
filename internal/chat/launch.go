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
