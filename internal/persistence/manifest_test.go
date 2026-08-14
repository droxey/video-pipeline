package persistence_test

import (
	"testing"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/persistence"
	"github.com/nebula/course-video-pipeline/internal/pipeline"
)

func TestSaveLoadManifest(t *testing.T) {
	dir := t.TempDir()
	m := pipeline.NewManifest("run-m1", "rec-001", "chash", "cfghash")

	if err := persistence.SaveManifest(dir, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	got, err := persistence.LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got.RunID != "run-m1" {
		t.Errorf("RunID = %q", got.RunID)
	}
	if got.ContentHash != "chash" {
		t.Errorf("ContentHash = %q", got.ContentHash)
	}
	if len(got.Stages) != len(domain.StageOrder) {
		t.Errorf("Stages count = %d, want %d", len(got.Stages), len(domain.StageOrder))
	}
}

func TestLoadManifest_NotFound(t *testing.T) {
	_, err := persistence.LoadManifest(t.TempDir() + "/nonexistent")
	if err == nil {
		t.Error("expected error loading missing manifest")
	}
}

func TestSaveLoadApproval(t *testing.T) {
	dir := t.TempDir()
	rec := &domain.ApprovalRecord{
		Gate:       "soup_validation",
		Approved:   true,
		ApprovedBy: "alice",
		ApprovedAt: time.Now().UTC(),
		Rationale:  "approved after review",
	}
	if err := persistence.SaveApproval(dir, rec); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	got, err := persistence.LoadApproval(dir, "soup_validation")
	if err != nil {
		t.Fatalf("LoadApproval: %v", err)
	}
	if !got.Approved {
		t.Error("expected Approved=true")
	}
	if got.ApprovedBy != "alice" {
		t.Errorf("ApprovedBy = %q", got.ApprovedBy)
	}
}

func TestWriteRejection(t *testing.T) {
	dir := t.TempDir()
	rec := &domain.RejectionRecord{
		RecordingID: "rec-001",
		RunID:       "run-x1",
		Stage:       domain.StageParse,
		Reasons:     []string{"placeholder density too high", "narrator coverage below threshold"},
		RejectedAt:  time.Now().UTC(),
	}
	if err := persistence.WriteRejection(dir, rec); err != nil {
		t.Fatalf("WriteRejection: %v", err)
	}
	var got domain.RejectionRecord
	if err := persistence.ReadJSON(persistence.ManifestPath(dir)[:len(persistence.ManifestPath(dir))-len("manifests/run.json")]+"rejected.json", &got); err != nil {
		// Try direct path.
	}
	// Verify the file exists and is valid JSON by re-reading it.
	path := dir + "/rejected.json"
	var check domain.RejectionRecord
	if err := persistence.ReadJSON(path, &check); err != nil {
		t.Fatalf("ReadJSON rejected.json: %v", err)
	}
	if check.RunID != "run-x1" {
		t.Errorf("RunID = %q", check.RunID)
	}
	if len(check.Reasons) != 2 {
		t.Errorf("Reasons count = %d", len(check.Reasons))
	}
}

func TestAppendReadUsage(t *testing.T) {
	dir := t.TempDir()
	rec := &domain.UsageRecord{
		Capability: "tts",
		RequestID:  "req-001",
		Timestamp:  time.Now().UTC(),
		Model:      "eleven_v3",
		Voice:      "voice-abc",
		InputChars: 512,
		CostStatus: domain.CostReported,
	}
	if err := persistence.AppendUsage(dir, rec); err != nil {
		t.Fatalf("AppendUsage: %v", err)
	}
	records, err := persistence.ReadUsage(dir)
	if err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Capability != "tts" {
		t.Errorf("Capability = %q", records[0].Capability)
	}
}

func TestReadUsage_Empty(t *testing.T) {
	records, err := persistence.ReadUsage(t.TempDir())
	if err != nil {
		t.Fatalf("ReadUsage empty: %v", err)
	}
	if records != nil {
		t.Error("expected nil for missing usage file")
	}
}
