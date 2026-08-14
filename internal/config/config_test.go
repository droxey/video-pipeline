package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/config"
	"github.com/nebula/course-video-pipeline/internal/domain"
)

func TestLoad_ValidYAML(t *testing.T) {
	cfg, err := config.Load("../../testdata/fixtures/course.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Slug != "test-course" {
		t.Errorf("slug = %q, want %q", cfg.Slug, "test-course")
	}
	if cfg.FPS != 30 {
		t.Errorf("fps = %d, want 30", cfg.FPS)
	}
	if cfg.PVCVoiceID != "voice-test-abc123" {
		t.Errorf("pvc_voice_id = %q", cfg.PVCVoiceID)
	}
	if !cfg.Soup.ValidationEnabled {
		t.Error("expected soup.validation_enabled=true")
	}
	if cfg.Soup.MinConfidence != 0.75 {
		t.Errorf("soup.min_confidence = %v, want 0.75", cfg.Soup.MinConfidence)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	_, err := config.Load("../../testdata/fixtures/course_invalid.yaml")
	if err == nil {
		t.Fatal("expected validation error for invalid config, got nil")
	}
}

func TestLoad_NotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/missing/course.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_JSON(t *testing.T) {
	const jsonData = `{
		"schema_version": 1,
		"slug": "json-course",
		"profile": "adult",
		"language": "en",
		"pvc_voice_id": "voice-json-xyz",
		"fps": 30,
		"width": 1920,
		"height": 1080,
		"soup": {"validation_enabled": true, "min_confidence": 0.75, "borderline_threshold": 0.55}
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "course.json")
	if err := os.WriteFile(path, []byte(jsonData), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load JSON: %v", err)
	}
	if cfg.Slug != "json-course" {
		t.Errorf("slug = %q, want %q", cfg.Slug, "json-course")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := validConfig()
	if err := config.Validate(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_WrongSchemaVersion(t *testing.T) {
	cfg := validConfig()
	cfg.SchemaVersion = 2
	if err := config.Validate(cfg); err == nil {
		t.Error("expected error for schema_version=2")
	}
}

func TestValidate_MissingSlug(t *testing.T) {
	cfg := validConfig()
	cfg.Slug = ""
	if err := config.Validate(cfg); err == nil {
		t.Error("expected error for empty slug")
	}
}

func TestValidate_MissingVoice(t *testing.T) {
	cfg := validConfig()
	cfg.PVCVoiceID = ""
	if err := config.Validate(cfg); err == nil {
		t.Error("expected error for missing pvc_voice_id")
	}
}

func TestValidate_InvalidFPS(t *testing.T) {
	cfg := validConfig()
	cfg.FPS = 0
	if err := config.Validate(cfg); err == nil {
		t.Error("expected error for fps=0")
	}
}

func TestValidate_SoupThresholds(t *testing.T) {
	tests := []struct {
		name      string
		min       float64
		threshold float64
		wantErr   bool
	}{
		{"valid", 0.75, 0.55, false},
		{"threshold_equals_min", 0.75, 0.75, true},
		{"threshold_above_min", 0.75, 0.80, true},
		{"zero_min", 0.0, 0.55, true},
		{"above_1_min", 1.1, 0.55, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Soup.ValidationEnabled = true
			cfg.Soup.MinConfidence = tt.min
			cfg.Soup.BorderlineThreshold = tt.threshold
			err := config.Validate(cfg)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidate_MusicDuration(t *testing.T) {
	cfg := validConfig()
	cfg.Music.Enabled = true
	cfg.Music.DurationSeconds = 0
	if err := config.Validate(cfg); err == nil {
		t.Error("expected error for music enabled with zero duration")
	}
}

// validConfig returns a minimal valid CourseConfig for test mutation.
func validConfig() *domain.CourseConfig {
	return &domain.CourseConfig{
		SchemaVersion: 1,
		Slug:          "test-course",
		Profile:       "adult",
		Language:      "en",
		PVCVoiceID:    "voice-test-abc",
		FPS:           30,
		Width:         1920,
		Height:        1080,
		Soup: domain.SoupCfg{
			ValidationEnabled:   true,
			MinConfidence:       0.75,
			BorderlineThreshold: 0.55,
		},
	}
}
