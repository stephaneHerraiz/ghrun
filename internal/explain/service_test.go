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
	st := &fakeStore{}
	svc := NewService(&fakeEmbedder{vec: []float32{1}}, &Chain{Explainers: []Explainer{ex}}, st,
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
	// Cache correctness: the memorized signature must be computed on the FULL
	// log (what ResolveLocal will see next time), not the truncated one.
	_, wantSig := Prepare(long)
	if len(st.upserts) != 1 || st.upserts[0].Signature != wantSig {
		t.Errorf("upsert must sign the full log: %+v", st.upserts)
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
