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
