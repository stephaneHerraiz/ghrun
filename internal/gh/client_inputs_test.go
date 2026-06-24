package gh

import (
	"encoding/base64"
	"errors"
	"testing"
)

const sampleWorkflow = `
name: Deploy
on:
  workflow_dispatch:
    inputs:
      environment:
        description: "Target env"
        type: choice
        required: true
        options: [staging, production]
      version:
        type: string
        default: "1.0.0"
      dry_run:
        type: boolean
        default: false
jobs:
  deploy:
    runs-on: ubuntu-latest
`

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestWorkflowInputsParses(t *testing.T) {
	f := (&fakeRunner{}).push(b64(sampleWorkflow)+"\n", nil)
	c := NewClient(f)

	inputs, err := c.WorkflowInputs(RepoRef{"o", "r"}, ".github/workflows/deploy.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 3 {
		t.Fatalf("got %d inputs: %+v", len(inputs), inputs)
	}
	// Order preserved.
	if inputs[0].Name != "environment" || inputs[2].Name != "dry_run" {
		t.Fatalf("order wrong: %+v", inputs)
	}
	if inputs[0].Type != InputChoice || !inputs[0].Required {
		t.Errorf("env input = %+v", inputs[0])
	}
	if len(inputs[0].Options) != 2 || inputs[0].Options[1] != "production" {
		t.Errorf("options = %v", inputs[0].Options)
	}
	if inputs[1].Default != "1.0.0" {
		t.Errorf("version default = %q", inputs[1].Default)
	}
	if inputs[2].Type != InputBoolean || inputs[2].Default != "false" {
		t.Errorf("dry_run = %+v", inputs[2])
	}
	// Verify the API call shape.
	got := f.lastCall()
	if got[0] != "api" || got[1] != "repos/o/r/contents/.github/workflows/deploy.yml" {
		t.Errorf("argv = %v", got)
	}
}

func TestWorkflowInputsNoDispatch(t *testing.T) {
	const wf = "name: CI\non:\n  push:\n    branches: [main]\njobs:\n  build:\n    runs-on: ubuntu-latest\n"
	f := (&fakeRunner{}).push(b64(wf), nil)
	c := NewClient(f)
	_, err := c.WorkflowInputs(RepoRef{"o", "r"}, ".github/workflows/ci.yml")
	if !errors.Is(err, ErrNoDispatch) {
		t.Fatalf("err = %v, want ErrNoDispatch", err)
	}
}
