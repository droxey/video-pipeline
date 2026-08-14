package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

const (
	manifestFile   = "run.json"
	approvalSubdir = "approvals"
	rejectionFile  = "rejected.json"
	usageJSONL     = "usage.jsonl"
)

// ManifestPath returns the canonical path for the run manifest.
func ManifestPath(runDir string) string {
	return filepath.Join(runDir, "manifests", manifestFile)
}

// LoadManifest reads and deserializes the run manifest from runDir.
func LoadManifest(runDir string) (*domain.Manifest, error) {
	var m domain.Manifest
	if err := ReadJSON(ManifestPath(runDir), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// SaveManifest atomically writes m to runDir/manifests/run.json, updating UpdatedAt.
func SaveManifest(runDir string, m *domain.Manifest) error {
	m.UpdatedAt = time.Now().UTC()
	dir := filepath.Join(runDir, "manifests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("persistence: mkdir %s: %w", dir, err)
	}
	return WriteJSON(ManifestPath(runDir), m)
}

// ApprovalPath returns the canonical path for a named gate's approval record.
func ApprovalPath(runDir, gate string) string {
	return filepath.Join(runDir, "manifests", approvalSubdir, gate+".json")
}

// LoadApproval reads the approval record for the named gate from runDir.
func LoadApproval(runDir, gate string) (*domain.ApprovalRecord, error) {
	var rec domain.ApprovalRecord
	if err := ReadJSON(ApprovalPath(runDir, gate), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// SaveApproval atomically writes rec to the approvals directory.
func SaveApproval(runDir string, rec *domain.ApprovalRecord) error {
	dir := filepath.Join(runDir, "manifests", approvalSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("persistence: mkdir %s: %w", dir, err)
	}
	return WriteJSON(ApprovalPath(runDir, rec.Gate), rec)
}

// WriteRejection writes rec atomically to runDir/rejected.json.
// The rejection file records the reason(s) a run was hard-rejected so
// that no paid provider calls are retried.
func WriteRejection(runDir string, rec *domain.RejectionRecord) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("persistence: mkdir %s: %w", runDir, err)
	}
	return WriteJSON(filepath.Join(runDir, rejectionFile), rec)
}

// AppendUsage appends a usage record to the per-run JSONL accounting file.
func AppendUsage(runDir string, rec *domain.UsageRecord) error {
	dir := filepath.Join(runDir, "manifests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("persistence: mkdir %s: %w", dir, err)
	}
	return appendJSONL(filepath.Join(dir, usageJSONL), rec)
}

// ReadUsage reads all usage records from the per-run JSONL file.
// Returns an empty slice when the file does not yet exist.
func ReadUsage(runDir string) ([]*domain.UsageRecord, error) {
	path := filepath.Join(runDir, "manifests", usageJSONL)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []*domain.UsageRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("persistence: open %s: %w", path, err)
	}
	defer f.Close()
	return scanJSONL[domain.UsageRecord](path, f)
}
