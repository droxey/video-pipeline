package pipeline

import (
	"fmt"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

// stageIndex maps each stage name to its 0-based position in StageOrder.
var stageIndex map[domain.Stage]int

func init() {
	stageIndex = make(map[domain.Stage]int, len(domain.StageOrder))
	for i, s := range domain.StageOrder {
		stageIndex[s] = i
	}
}

// NewManifest constructs a fresh Manifest for a new run with all stages in pending state.
func NewManifest(runID, recordingID, contentHash, configHash string) *domain.Manifest {
	now := time.Now().UTC()
	m := &domain.Manifest{
		RunID:       runID,
		RecordingID: recordingID,
		ContentHash: contentHash,
		ConfigHash:  configHash,
		Status:      domain.ManifestRunning,
		Stages:      make(map[domain.Stage]*domain.StageState, len(domain.StageOrder)),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for _, s := range domain.StageOrder {
		m.Stages[s] = &domain.StageState{Stage: s, Status: domain.StagePending}
	}
	return m
}

// Transition moves stage s to the given status in m, enforcing fixed ordering and
// valid status transitions. It does not persist the manifest.
func Transition(m *domain.Manifest, s domain.Stage, status domain.StageStatus) error {
	idx, ok := stageIndex[s]
	if !ok {
		return fmt.Errorf("stages: unknown stage %q", s)
	}

	state, exists := m.Stages[s]
	if !exists {
		return fmt.Errorf("stages: stage %q missing from manifest", s)
	}

	// Before running or completing a stage, all predecessors must be done.
	if status == domain.StageRunning || status == domain.StageDone {
		for i := 0; i < idx; i++ {
			pred := domain.StageOrder[i]
			if m.Stages[pred].Status != domain.StageDone {
				return fmt.Errorf("stages: cannot start %q before %q is done (status=%s)",
					s, pred, m.Stages[pred].Status)
			}
		}
	}

	// Enforce valid state machine transitions.
	switch state.Status {
	case domain.StagePending:
		if status != domain.StageRunning {
			return fmt.Errorf("stages: %q: pending → %q is not allowed (must go to running first)", s, status)
		}
	case domain.StageRunning:
		if status != domain.StageDone && status != domain.StageFailed && status != domain.StageNeedsReview {
			return fmt.Errorf("stages: %q: running → %q is not allowed", s, status)
		}
	case domain.StageFailed:
		// Allow retry.
		if status != domain.StageRunning {
			return fmt.Errorf("stages: %q: failed → %q is not allowed (only retry to running)", s, status)
		}
	case domain.StageNeedsReview:
		// Only human resolution can advance from here.
		if status != domain.StageDone && status != domain.StageFailed {
			return fmt.Errorf("stages: %q: needs_review → %q is not allowed (only done|failed via review resolve)", s, status)
		}
	case domain.StageDone:
		return fmt.Errorf("stages: %q is already done; create a new run to re-execute", s)
	default:
		return fmt.Errorf("stages: %q: unhandled current status %q", s, state.Status)
	}

	now := time.Now().UTC()
	state.Status = status
	switch status {
	case domain.StageRunning:
		state.StartedAt = &now
		state.DoneAt = nil
		state.Error = ""
	case domain.StageDone:
		state.DoneAt = &now
	}
	m.UpdatedAt = now
	return nil
}

// MarkFailed records a hard failure on stage s, setting its error and moving
// the overall manifest to failed. Safe to call even when the stage is not running.
func MarkFailed(m *domain.Manifest, s domain.Stage, reason string) {
	state, ok := m.Stages[s]
	if !ok {
		return
	}
	now := time.Now().UTC()
	state.Status = domain.StageFailed
	state.Error = reason
	state.DoneAt = &now
	m.Status = domain.ManifestFailed
	m.UpdatedAt = now
}

// ResumePoint returns the first stage in StageOrder that is not yet done,
// and true. When every stage is done it returns ("", false).
func ResumePoint(m *domain.Manifest) (domain.Stage, bool) {
	for _, s := range domain.StageOrder {
		if m.Stages[s].Status != domain.StageDone {
			return s, true
		}
	}
	return "", false
}

// CanResume checks that the manifest's recorded hashes match the provided
// content and config hashes. Returns a descriptive error on mismatch so the
// caller can surface it rather than silently running with stale data.
func CanResume(m *domain.Manifest, contentHash, configHash string) error {
	if m.ContentHash != contentHash {
		return fmt.Errorf("stages: content hash mismatch: manifest=%s current=%s (source content changed)",
			m.ContentHash, contentHash)
	}
	if m.ConfigHash != configHash {
		return fmt.Errorf("stages: config hash mismatch: manifest=%s current=%s (course.yaml changed)",
			m.ConfigHash, configHash)
	}
	return nil
}
