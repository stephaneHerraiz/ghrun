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

// NewOllamaEmbedder builds an embedder for POST {baseURL}/api/embeddings.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		httpc:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{"model": o.model, "prompt": text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embeddings", bytes.NewReader(body))
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
	var out struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama: decoding response: %w", err)
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding (is model %q pulled?)", o.model)
	}
	return out.Embedding, nil
}
