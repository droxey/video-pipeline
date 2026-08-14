package pipeline_test

import (
	"testing"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/pipeline"
)

func TestNewManifest_AllStagesPending(t *testing.T) {
	m := pipeline.NewManifest("run-001", "rec-001", "chash", "cfghash")
	if m.RunID != "run-001" {
		t.Errorf("RunID = %q", m.RunID)
	}
	if m.Status != domain.ManifestRunning {
		t.Errorf("Status = %q, want running", m.Status)
	}
	for _, s := range domain.StageOrder {
		st, ok := m.Stages[s]
		if !ok {
			t.Errorf("stage %q missing from manifest", s)
			continue
		}
		if st.Status != domain.StagePending {
			t.Errorf("stage %q status = %q, want pending", s, st.Status)
		}
	}
}

func TestTransition_HappyPath(t *testing.T) {
	m := pipeline.NewManifest("run-002", "rec-001", "ch", "cfg")

	// Walk through the first two stages in order.
	for _, s := range domain.StageOrder[:2] {
		if err := pipeline.Transition(m, s, domain.StageRunning); err != nil {
			t.Fatalf("Transition %q → running: %v", s, err)
		}
		if err := pipeline.Transition(m, s, domain.StageDone); err != nil {
			t.Fatalf("Transition %q → done: %v", s, err)
		}
		if m.Stages[s].Status != domain.StageDone {
			t.Errorf("stage %q = %q, want done", s, m.Stages[s].Status)
		}
	}
}

func TestTransition_RejectsOutOfOrder(t *testing.T) {
	m := pipeline.NewManifest("run-003", "rec-001", "ch", "cfg")
	// Try to run stage[1] without completing stage[0].
	s1 := domain.StageOrder[1]
	err := pipeline.Transition(m, s1, domain.StageRunning)
	if err == nil {
		t.Errorf("expected error running %q before predecessors are done", s1)
	}
}

func TestTransition_RejectsPendingToDone(t *testing.T) {
	m := pipeline.NewManifest("run-004", "rec-001", "ch", "cfg")
	s := domain.StageOrder[0]
	err := pipeline.Transition(m, s, domain.StageDone)
	if err == nil {
		t.Error("expected error transitioning pending → done (must go through running)")
	}
}

func TestTransition_RejectsDoneStage(t *testing.T) {
	m := pipeline.NewManifest("run-005", "rec-001", "ch", "cfg")
	s := domain.StageOrder[0]
	pipeline.Transition(m, s, domain.StageRunning)
	pipeline.Transition(m, s, domain.StageDone)
	err := pipeline.Transition(m, s, domain.StageRunning)
	if err == nil {
		t.Error("expected error retrying a done stage")
	}
}

func TestTransition_RetryFromFailed(t *testing.T) {
	m := pipeline.NewManifest("run-006", "rec-001", "ch", "cfg")
	s := domain.StageOrder[0]
	pipeline.Transition(m, s, domain.StageRunning)
	pipeline.Transition(m, s, domain.StageFailed)
	// Retry is allowed.
	if err := pipeline.Transition(m, s, domain.StageRunning); err != nil {
		t.Errorf("expected retry from failed to succeed: %v", err)
	}
}

func TestTransition_NeedsReviewCanResolve(t *testing.T) {
	m := pipeline.NewManifest("run-007", "rec-001", "ch", "cfg")
	s := domain.StageOrder[0]
	pipeline.Transition(m, s, domain.StageRunning)
	pipeline.Transition(m, s, domain.StageNeedsReview)

	// Can resolve to done.
	if err := pipeline.Transition(m, s, domain.StageDone); err != nil {
		t.Errorf("needs_review → done: %v", err)
	}
}

func TestTransition_UnknownStage(t *testing.T) {
	m := pipeline.NewManifest("run-008", "rec-001", "ch", "cfg")
	err := pipeline.Transition(m, domain.Stage("nonexistent"), domain.StageRunning)
	if err == nil {
		t.Error("expected error for unknown stage")
	}
}

func TestResumePoint_ReturnsFirstNonDone(t *testing.T) {
	m := pipeline.NewManifest("run-009", "rec-001", "ch", "cfg")
	// Complete first stage only.
	pipeline.Transition(m, domain.StageOrder[0], domain.StageRunning)
	pipeline.Transition(m, domain.StageOrder[0], domain.StageDone)

	s, ok := pipeline.ResumePoint(m)
	if !ok {
		t.Fatal("expected a resume point")
	}
	if s != domain.StageOrder[1] {
		t.Errorf("ResumePoint = %q, want %q", s, domain.StageOrder[1])
	}
}

func TestResumePoint_AllDone(t *testing.T) {
	m := pipeline.NewManifest("run-010", "rec-001", "ch", "cfg")
	for _, s := range domain.StageOrder {
		pipeline.Transition(m, s, domain.StageRunning)
		pipeline.Transition(m, s, domain.StageDone)
	}
	_, ok := pipeline.ResumePoint(m)
	if ok {
		t.Error("expected no resume point when all stages are done")
	}
}

func TestCanResume_Match(t *testing.T) {
	m := pipeline.NewManifest("run-011", "rec-001", "abc123", "def456")
	if err := pipeline.CanResume(m, "abc123", "def456"); err != nil {
		t.Errorf("CanResume with matching hashes: %v", err)
	}
}

func TestCanResume_ContentMismatch(t *testing.T) {
	m := pipeline.NewManifest("run-012", "rec-001", "original", "cfg")
	if err := pipeline.CanResume(m, "changed", "cfg"); err == nil {
		t.Error("expected error on content hash mismatch")
	}
}

func TestCanResume_ConfigMismatch(t *testing.T) {
	m := pipeline.NewManifest("run-013", "rec-001", "content", "original-cfg")
	if err := pipeline.CanResume(m, "content", "new-cfg"); err == nil {
		t.Error("expected error on config hash mismatch")
	}
}

func TestMarkFailed_SetsManifestStatus(t *testing.T) {
	m := pipeline.NewManifest("run-014", "rec-001", "ch", "cfg")
	s := domain.StageOrder[0]
	pipeline.Transition(m, s, domain.StageRunning)
	pipeline.MarkFailed(m, s, "network timeout")

	if m.Stages[s].Status != domain.StageFailed {
		t.Errorf("stage status = %q, want failed", m.Stages[s].Status)
	}
	if m.Stages[s].Error != "network timeout" {
		t.Errorf("error = %q, want %q", m.Stages[s].Error, "network timeout")
	}
	if m.Status != domain.ManifestFailed {
		t.Errorf("manifest status = %q, want failed", m.Status)
	}
}
