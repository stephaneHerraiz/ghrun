package explain

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	chromem "github.com/philippgille/chromem-go"
)

const collectionName = "explanations"

// ChromemStore persists entries in an embedded chromem-go database.
type ChromemStore struct {
	coll *chromem.Collection
}

// NewChromemStore opens (or creates) the persistent store at path. A
// corrupted or unreadable store is wiped and recreated empty: losing the
// cache must never break the feature.
func NewChromemStore(path string) (*ChromemStore, error) {
	coll, err := openCollection(path)
	if err != nil {
		if rmErr := os.RemoveAll(path); rmErr != nil {
			return nil, errors.Join(err, rmErr)
		}
		coll, err = openCollection(path)
		if err != nil {
			return nil, err
		}
	}
	return &ChromemStore{coll: coll}, nil
}

func openCollection(path string) (*chromem.Collection, error) {
	db, err := chromem.NewPersistentDB(path, false)
	if err != nil {
		return nil, err
	}
	return db.GetOrCreateCollection(collectionName, nil, noEmbedding)
}

// noEmbedding guards the collection: embeddings are always provided
// explicitly, chromem must never compute one itself.
func noEmbedding(context.Context, string) ([]float32, error) {
	return nil, errors.New("explain: embedding function should never be called")
}

func (s *ChromemStore) GetBySignature(sig string) (*Entry, bool) {
	doc, err := s.coll.GetByID(context.Background(), sig)
	if err != nil {
		return nil, false
	}
	e := docToEntry(doc.ID, doc.Content, doc.Embedding, doc.Metadata)
	return &e, true
}

func (s *ChromemStore) Query(embedding []float32, topK int) ([]Match, error) {
	n := s.coll.Count()
	if n == 0 || topK <= 0 {
		return nil, nil
	}
	if topK > n {
		topK = n // chromem errors when nResults > document count
	}
	results, err := s.coll.QueryEmbedding(context.Background(), embedding, topK, nil, nil)
	if err != nil {
		return nil, err
	}
	matches := make([]Match, 0, len(results))
	for _, r := range results {
		matches = append(matches, Match{
			Entry:      docToEntry(r.ID, r.Content, r.Embedding, r.Metadata),
			Similarity: r.Similarity,
		})
	}
	return matches, nil
}

func (s *ChromemStore) Upsert(e Entry) error {
	// AddDocument overwrites an existing ID: native upsert.
	return s.coll.AddDocument(context.Background(), chromem.Document{
		ID:        e.Signature,
		Content:   e.Normalized,
		Embedding: e.Embedding,
		Metadata:  entryMetadata(e),
	})
}

func (s *ChromemStore) Touch(sig string) error {
	doc, err := s.coll.GetByID(context.Background(), sig)
	if err != nil {
		return err
	}
	doc.Metadata["useCount"] = strconv.Itoa(atoiOr(doc.Metadata["useCount"], 0) + 1)
	doc.Metadata["lastUsedAt"] = time.Now().UTC().Format(time.RFC3339)
	return s.coll.AddDocument(context.Background(), doc)
}

func entryMetadata(e Entry) map[string]string {
	return map[string]string{
		"explanation": e.Explanation,
		"repo":        e.Repo,
		"workflow":    e.Workflow,
		"failedSteps": e.FailedSteps,
		"model":       e.Model,
		"createdAt":   e.CreatedAt.UTC().Format(time.RFC3339),
		"lastUsedAt":  e.LastUsedAt.UTC().Format(time.RFC3339),
		"useCount":    strconv.Itoa(e.UseCount),
		"language":    e.Language,
	}
}

func docToEntry(id, content string, embedding []float32, meta map[string]string) Entry {
	createdAt, _ := time.Parse(time.RFC3339, meta["createdAt"])
	lastUsedAt, _ := time.Parse(time.RFC3339, meta["lastUsedAt"])
	return Entry{
		Signature:   id,
		Normalized:  content,
		Embedding:   embedding,
		Explanation: meta["explanation"],
		Repo:        meta["repo"],
		Workflow:    meta["workflow"],
		FailedSteps: meta["failedSteps"],
		Model:       meta["model"],
		CreatedAt:   createdAt,
		LastUsedAt:  lastUsedAt,
		UseCount:    atoiOr(meta["useCount"], 0),
		Language:    meta["language"],
	}
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
