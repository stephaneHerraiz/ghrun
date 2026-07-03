package explain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
)

func anthropicOK(t *testing.T, capture *map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			_ = json.NewDecoder(r.Body).Decode(capture)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model": "claude-sonnet-5",
			"content": [{"type": "text", "text": "Root cause: missing dependency."}],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}
}

func TestAnthropicExplainerSendsPromptAndParsesText(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(anthropicOK(t, &got))
	defer srv.Close()

	e := NewAnthropicExplainer("test-key", "claude-sonnet-5",
		option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	out, err := e.Explain(context.Background(), ExplainRequest{
		Log: "Error: cannot find module", Repo: "o/r", Workflow: "CI", Language: "English",
	})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if out != "Root cause: missing dependency." {
		t.Errorf("out = %q", out)
	}
	if got["model"] != "claude-sonnet-5" {
		t.Errorf("model = %v", got["model"])
	}
	if got["max_tokens"] != float64(2048) {
		t.Errorf("max_tokens = %v", got["max_tokens"])
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "GitHub Actions run failed") {
		t.Errorf("system prompt missing from request: %s", raw)
	}
	if !strings.Contains(string(raw), "cannot find module") {
		t.Errorf("log missing from request: %s", raw)
	}
	if e.Name() != "claude-sonnet-5" {
		t.Errorf("Name() = %q", e.Name())
	}
}

func TestAnthropicExplainerServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"type":"error","error":{"type":"api_error","message":"boom"}}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	e := NewAnthropicExplainer("test-key", "claude-sonnet-5",
		option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	if _, err := e.Explain(context.Background(), ExplainRequest{Log: "x"}); err == nil {
		t.Error("want error on 500")
	}
}
