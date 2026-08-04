package explain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOllamaEmbedderPostsAndDecodes(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3]]}`))
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL+"/", "nomic-embed-text") // trailing slash must be tolerated
	vec, err := e.Embed(context.Background(), "Error: boom")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 3 || vec[1] != 0.2 {
		t.Errorf("vec = %v", vec)
	}
	if gotPath != "/api/embed" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["model"] != "nomic-embed-text" || gotBody["input"] != "Error: boom" {
		t.Errorf("body = %v", gotBody)
	}
}

// A failed CI log routinely exceeds the embedding model's context window
// (nomic-embed-text: 2048 tokens). The embedder MUST ask Ollama to truncate
// server-side, otherwise Ollama returns 500 "input length exceeds the context
// length" and the whole knowledge base is wrongly reported as unreachable.
func TestOllamaEmbedderRequestsServerSideTruncation(t *testing.T) {
	var gotTruncate any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotTruncate = body["truncate"]
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()
	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	if _, err := e.Embed(context.Background(), strings.Repeat("boom ", 100000)); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotTruncate != true {
		t.Errorf("truncate flag = %v, want true", gotTruncate)
	}
}

// Ollama's server-side truncation is unreliable: for a real normalized CI log
// (hundreds of KB) it answers 400 "input length exceeds the context length"
// even with truncate:true. The embedder must therefore cap the input itself,
// keeping the tail (where the conclusive error lives), so it always fits the
// model's context window.
func TestOllamaEmbedderTruncatesOversizedInput(t *testing.T) {
	var gotInput string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotInput, _ = body["input"].(string)
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()
	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	long := strings.Repeat("noise\n", 50000) + "THE CONCLUSIVE ERROR"
	if _, err := e.Embed(context.Background(), long); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if n := utf8.RuneCountInString(gotInput); n > maxEmbedRunes {
		t.Errorf("input not capped: %d runes (max %d)", n, maxEmbedRunes)
	}
	if !strings.HasSuffix(gotInput, "THE CONCLUSIVE ERROR") {
		t.Errorf("truncation must keep the tail, got suffix %q", gotInput[max(0, len(gotInput)-40):])
	}
}

func TestOllamaEmbedderKeepsShortInputIntact(t *testing.T) {
	var gotInput string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotInput, _ = body["input"].(string)
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()
	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	if _, err := e.Embed(context.Background(), "Error: boom"); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotInput != "Error: boom" {
		t.Errorf("short input altered: %q", gotInput)
	}
}

func TestOllamaEmbedderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()
	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	if _, err := e.Embed(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("want 404 error, got %v", err)
	}
}

func TestOllamaEmbedderEmptyEmbedding(t *testing.T) {
	for _, payload := range []string{`{"embeddings":[[]]}`, `{"embeddings":[]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(payload))
		}))
		e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
		if _, err := e.Embed(context.Background(), "x"); err == nil {
			t.Errorf("want error on empty embedding for %s", payload)
		}
		srv.Close()
	}
}

func TestPrompts(t *testing.T) {
	sys := SystemPrompt("English")
	if !strings.Contains(sys, "GitHub Actions") || !strings.Contains(sys, "Answer in English") {
		t.Errorf("system prompt = %q", sys)
	}
	if got := SystemPrompt(""); !strings.Contains(got, "Answer in English") {
		t.Errorf("empty language must default to English, got %q", got)
	}
	up := UserPrompt(ExplainRequest{
		Log: "raw log", Repo: "o/r", Workflow: "CI",
		FailedSteps: []string{"build / test"},
	})
	for _, want := range []string{"o/r", "CI", "build / test", "raw log"} {
		if !strings.Contains(up, want) {
			t.Errorf("user prompt %q misses %q", up, want)
		}
	}
}
