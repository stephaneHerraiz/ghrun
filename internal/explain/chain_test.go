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
