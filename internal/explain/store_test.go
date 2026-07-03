package explain

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testEntry(sig string) Entry {
	return Entry{
		Signature:   sig,
		Normalized:  "build <TS> Error: undefined symbol",
		Embedding:   []float32{1, 0, 0},
		Explanation: "The linker failed because the symbol is undefined.",
		Repo:        "o/r",
		Workflow:    "CI",
		FailedSteps: "build / link",
		Model:       "claude-sonnet-5",
		CreatedAt:   time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC),
		LastUsedAt:  time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC),
		UseCount:    1,
		Language:    "English",
	}
}

func TestStoreUpsertAndGetBySignature(t *testing.T) {
	s, err := NewChromemStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	if _, ok := s.GetBySignature("missing"); ok {
		t.Fatal("empty store returned an entry")
	}
	want := testEntry("sig-1")
	if err := s.Upsert(want); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, ok := s.GetBySignature("sig-1")
	if !ok {
		t.Fatal("entry not found after upsert")
	}
	if got.Explanation != want.Explanation || got.Repo != "o/r" || got.UseCount != 1 ||
		got.Model != "claude-sonnet-5" || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("got %+v", got)
	}
}

func TestStoreQueryEmptyAndSimilarity(t *testing.T) {
	s, err := NewChromemStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	// Empty store: no error, no matches (chromem errors when nResults > count).
	if m, err := s.Query([]float32{1, 0, 0}, 1); err != nil || len(m) != 0 {
		t.Fatalf("empty query: m=%v err=%v", m, err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	m, err := s.Query([]float32{1, 0, 0}, 5) // topK beyond count must be clamped
	if err != nil || len(m) != 1 {
		t.Fatalf("query: m=%v err=%v", m, err)
	}
	if m[0].Similarity < 0.99 {
		t.Errorf("identical vector similarity = %f", m[0].Similarity)
	}
	if m[0].Explanation == "" || m[0].Signature != "sig-1" {
		t.Errorf("match entry not hydrated: %+v", m[0].Entry)
	}
}

func TestStoreTouchIncrements(t *testing.T) {
	s, err := NewChromemStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.Touch("sig-1"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, _ := s.GetBySignature("sig-1")
	if got.UseCount != 2 {
		t.Errorf("UseCount = %d, want 2", got.UseCount)
	}
	if !got.LastUsedAt.After(testEntry("sig-1").LastUsedAt) {
		t.Errorf("LastUsedAt not refreshed: %v", got.LastUsedAt)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	s, err := NewChromemStore(dir)
	if err != nil {
		t.Fatalf("NewChromemStore: %v", err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	s2, err := NewChromemStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok := s2.GetBySignature("sig-1"); !ok {
		t.Error("entry lost after reopen")
	}
}

func TestStoreCorruptedRecreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db")
	// A regular file where the DB directory should be makes chromem fail.
	if err := os.WriteFile(path, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewChromemStore(path)
	if err != nil {
		t.Fatalf("corrupted store not recreated: %v", err)
	}
	if err := s.Upsert(testEntry("sig-1")); err != nil {
		t.Errorf("Upsert after recreation: %v", err)
	}
}
