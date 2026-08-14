// Package pipeline provides deterministic naming, stage transitions, and resume logic.
package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

// ContentHash returns the lowercase hex SHA-256 digest of data.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ConfigHash returns the lowercase hex SHA-256 digest of the canonical JSON encoding of cfg.
func ConfigHash(cfg *domain.CourseConfig) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("hash: marshal config: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// RunID returns a deterministic run directory name derived from all three inputs.
// Format: {sanitized_recording_id}_{sha256(inputs)[:16]}
func RunID(recordingID, contentHash, configHash string) string {
	combined := recordingID + ":" + contentHash + ":" + configHash
	sum := sha256.Sum256([]byte(combined))
	shortHash := hex.EncodeToString(sum[:])[:16]
	return sanitizeID(recordingID) + "_" + shortHash
}

// sanitizeID replaces characters unsafe for file/directory names with underscores.
func sanitizeID(id string) string {
	var sb strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	return sb.String()
}
