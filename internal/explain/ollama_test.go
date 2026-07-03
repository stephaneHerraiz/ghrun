package explain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaEmbedderPostsAndDecodes(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"embedding":[0.1,0.2,0.3]}`))
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
	if gotPath != "/api/embeddings" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["model"] != "nomic-embed-text" || gotBody["prompt"] != "Error: boom" {
		t.Errorf("body = %v", gotBody)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"embedding":[]}`))
	}))
	defer srv.Close()
	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	if _, err := e.Embed(context.Background(), "x"); err == nil {
		t.Error("want error on empty embedding")
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
