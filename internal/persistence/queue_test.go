package persistence_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/persistence"
)

func makeEntry(id string, confidence float64) *domain.ReviewQueueEntry {
	return &domain.ReviewQueueEntry{
		ID:         id,
		PlanHash:   "planhash-" + id,
		Confidence: confidence,
		Verdict:    "PASS",
		Reasons:    []string{"good content", "clear structure"},
		Status:     domain.ReviewPending,
		CreatedAt:  time.Now().UTC(),
	}
}

func TestAppendRead_SingleEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review_queue.jsonl")

	e := makeEntry("entry-001", 0.68)
	if err := persistence.AppendReviewEntry(path, e); err != nil {
		t.Fatalf("AppendReviewEntry: %v", err)
	}

	entries, err := persistence.ReadReviewQueue(path)
	if err != nil {
		t.Fatalf("ReadReviewQueue: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "entry-001" {
		t.Errorf("ID = %q", entries[0].ID)
	}
	if entries[0].Confidence != 0.68 {
		t.Errorf("Confidence = %v", entries[0].Confidence)
	}
}

func TestAppendRead_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")

	for i, id := range []string{"a1b2c3d4", "e5f6g7h8", "i9j0k1l2"} {
		if err := persistence.AppendReviewEntry(path, makeEntry(id, float64(i)*0.1+0.5)); err != nil {
			t.Fatalf("AppendReviewEntry %s: %v", id, err)
		}
	}

	entries, err := persistence.ReadReviewQueue(path)
	if err != nil {
		t.Fatalf("ReadReviewQueue: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	ids := []string{"a1b2c3d4", "e5f6g7h8", "i9j0k1l2"}
	for i, e := range entries {
		if e.ID != ids[i] {
			t.Errorf("entry[%d].ID = %q, want %q", i, e.ID, ids[i])
		}
	}
}

func TestReadReviewQueue_MissingFile(t *testing.T) {
	entries, err := persistence.ReadReviewQueue("/nonexistent/queue.jsonl")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries for missing file, got %v", entries)
	}
}

func TestResolveReviewEntry_Approved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")

	persistence.AppendReviewEntry(path, makeEntry("aaa", 0.65))
	persistence.AppendReviewEntry(path, makeEntry("bbb", 0.70))

	if err := persistence.ResolveReviewEntry(path, "aaa", domain.ReviewApproved, "looks good"); err != nil {
		t.Fatalf("ResolveReviewEntry: %v", err)
	}

	entries, _ := persistence.ReadReviewQueue(path)
	for _, e := range entries {
		if e.ID == "aaa" {
			if e.Status != domain.ReviewApproved {
				t.Errorf("status = %q, want approved", e.Status)
			}
			if e.ReviewerNotes == nil || *e.ReviewerNotes != "looks good" {
				t.Errorf("reviewer_notes = %v", e.ReviewerNotes)
			}
		}
		if e.ID == "bbb" && e.Status != domain.ReviewPending {
			t.Errorf("bbb status should still be pending, got %q", e.Status)
		}
	}
}

func TestResolveReviewEntry_Rejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")
	persistence.AppendReviewEntry(path, makeEntry("zzz", 0.60))

	if err := persistence.ResolveReviewEntry(path, "zzz", domain.ReviewRejected, "not educational"); err != nil {
		t.Fatalf("ResolveReviewEntry: %v", err)
	}
	entries, _ := persistence.ReadReviewQueue(path)
	if entries[0].Status != domain.ReviewRejected {
		t.Errorf("status = %q, want rejected", entries[0].Status)
	}
}

func TestResolveReviewEntry_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")
	persistence.AppendReviewEntry(path, makeEntry("exists", 0.60))

	err := persistence.ResolveReviewEntry(path, "does-not-exist", domain.ReviewApproved, "")
	if err == nil {
		t.Error("expected error resolving non-existent ID")
	}
}

func TestResolveReviewEntry_PreservesOtherEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.jsonl")

	for _, id := range []string{"p1", "p2", "p3"} {
		persistence.AppendReviewEntry(path, makeEntry(id, 0.65))
	}
	persistence.ResolveReviewEntry(path, "p2", domain.ReviewApproved, "")

	entries, _ := persistence.ReadReviewQueue(path)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after resolve, got %d", len(entries))
	}
}

func TestAppendReviewEntry_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "queue.jsonl")
	e := makeEntry("new-dir-entry", 0.72)
	if err := persistence.AppendReviewEntry(path, e); err != nil {
		t.Fatalf("AppendReviewEntry with deep path: %v", err)
	}
	entries, _ := persistence.ReadReviewQueue(path)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after creating dirs, got %d", len(entries))
	}
}
