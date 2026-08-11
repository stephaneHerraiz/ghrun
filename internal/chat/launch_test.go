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
