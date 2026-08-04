package explain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaEmbedder implements Embedder against a local Ollama server.
type OllamaEmbedder struct {
	baseURL string
	model   string
	httpc   *http.Client
}

// NewOllamaEmbedder builds an embedder for POST {baseURL}/api/embed.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		httpc:   &http.Client{Timeout: 30 * time.Second},
	}
}

// maxEmbedRunes caps the text sent for embedding. Ollama's server-side
// truncation is unreliable — for a large input it answers "input length
// exceeds the context length" even with truncate:true — so we cap the input
// ourselves. A subword tokenizer never emits more tokens than characters, so
// keeping at most 2000 runes guarantees the input fits nomic-embed-text's
// 2048-token context regardless of content, without depending on the server.
// 2000 runes of the error-region tail is also a focused retrieval signature.
const maxEmbedRunes = 2000

func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// truncate:true is a harmless backstop; the real guarantee is our own cap
	// above, because Ollama's truncation is not reliable on large inputs.
	body, err := json.Marshal(map[string]any{"model": o.model, "input": tailRunes(text, maxEmbedRunes), "truncate": true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	// /api/embed returns a batch ("embeddings": [][]float32); a single string
	// input yields exactly one row.
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama: decoding response: %w", err)
	}
	if len(out.Embeddings) == 0 || len(out.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding (is model %q pulled?)", o.model)
	}
	return out.Embeddings[0], nil
}

// tailRunes returns the last n runes of s (s unchanged when already within n).
// When it truncates it drops everything up to the first newline so the result
// starts on a line boundary rather than mid-line.
func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[len(r)-n:])
	if i := strings.IndexByte(cut, '\n'); i >= 0 && i+1 < len(cut) {
		cut = cut[i+1:]
	}
	return cut
}
