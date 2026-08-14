package pipeline_test

import (
	"testing"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/pipeline"
)

func TestContentHash_Deterministic(t *testing.T) {
	data := []byte("hello, nebula pipeline")
	h1 := pipeline.ContentHash(data)
	h2 := pipeline.ContentHash(data)
	if h1 != h2 {
		t.Errorf("ContentHash not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got len=%d", len(h1))
	}
}

func TestContentHash_Distinct(t *testing.T) {
	h1 := pipeline.ContentHash([]byte("aaa"))
	h2 := pipeline.ContentHash([]byte("bbb"))
	if h1 == h2 {
		t.Error("different inputs must produce different hashes")
	}
}

func TestContentHash_Empty(t *testing.T) {
	h := pipeline.ContentHash(nil)
	if len(h) != 64 {
		t.Errorf("empty data hash len=%d, want 64", len(h))
	}
}

func TestConfigHash_Deterministic(t *testing.T) {
	cfg := minimalConfig()
	h1, err := pipeline.ConfigHash(cfg)
	if err != nil {
		t.Fatalf("ConfigHash: %v", err)
	}
	h2, err := pipeline.ConfigHash(cfg)
	if err != nil {
		t.Fatalf("ConfigHash: %v", err)
	}
	if h1 != h2 {
		t.Errorf("ConfigHash not deterministic: %q != %q", h1, h2)
	}
}

func TestConfigHash_ChangeDetected(t *testing.T) {
	cfg := minimalConfig()
	h1, _ := pipeline.ConfigHash(cfg)
	cfg.FPS = 60
	h2, _ := pipeline.ConfigHash(cfg)
	if h1 == h2 {
		t.Error("different configs must produce different hashes")
	}
}

func TestRunID_Deterministic(t *testing.T) {
	id1 := pipeline.RunID("grain-rec-00abc123", "contenthash", "confighash")
	id2 := pipeline.RunID("grain-rec-00abc123", "contenthash", "confighash")
	if id1 != id2 {
		t.Errorf("RunID not deterministic: %q != %q", id1, id2)
	}
}

func TestRunID_DifferentInputs(t *testing.T) {
	id1 := pipeline.RunID("grain-rec-00abc123", "hash1", "hash2")
	id2 := pipeline.RunID("grain-rec-00abc123", "hash1", "hash3")
	if id1 == id2 {
		t.Error("different config hashes must produce different run IDs")
	}
}

func TestRunID_ContainsRecordingID(t *testing.T) {
	id := pipeline.RunID("my-recording", "c1", "c2")
	if len(id) == 0 {
		t.Fatal("RunID must not be empty")
	}
	// The sanitized recording ID should appear as a prefix.
	if id[:len("my-recording")] != "my-recording" {
		t.Errorf("RunID %q should start with sanitized recording ID", id)
	}
}

func TestRunID_SpecialCharsInRecordingID(t *testing.T) {
	id := pipeline.RunID("grain/rec?id=42", "ch", "cfg")
	for _, r := range id[:16] {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		t.Errorf("RunID prefix contains unsafe char %q in %q", r, id)
	}
}

func minimalConfig() *domain.CourseConfig {
	return &domain.CourseConfig{
		SchemaVersion: 1,
		Slug:          "test-course",
		Language:      "en",
		PVCVoiceID:    "voice-abc",
		FPS:           30,
		Width:         1920,
		Height:        1080,
	}
}
