// Package config handles loading and validating course configuration files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"gopkg.in/yaml.v3"
)

// Load reads a CourseConfig from path. Supports .yaml/.yml and .json extensions.
// When the extension is unrecognized it tries YAML then JSON.
// Returns a validated config or an error.
func Load(path string) (*domain.CourseConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg domain.CourseConfig
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("config: parse YAML %s: %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("config: parse JSON %s: %w", path, err)
		}
	default:
		// Attempt YAML first, fall back to JSON.
		if yamlErr := yaml.Unmarshal(data, &cfg); yamlErr != nil {
			if jsonErr := json.Unmarshal(data, &cfg); jsonErr != nil {
				return nil, fmt.Errorf("config: unrecognized format in %s (YAML: %v; JSON: %v)", path, yamlErr, jsonErr)
			}
		}
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config: invalid %s: %w", path, err)
	}
	return &cfg, nil
}

// Validate checks that cfg satisfies all structural constraints defined by the spec.
func Validate(cfg *domain.CourseConfig) error {
	var errs []string

	if cfg.SchemaVersion != 1 {
		errs = append(errs, fmt.Sprintf("schema_version must be 1, got %d", cfg.SchemaVersion))
	}
	if strings.TrimSpace(cfg.Slug) == "" {
		errs = append(errs, "slug is required")
	}
	if strings.TrimSpace(cfg.Language) == "" {
		errs = append(errs, "language is required")
	}
	if strings.TrimSpace(cfg.PVCVoiceID) == "" {
		errs = append(errs, "pvc_voice_id is required")
	}
	if cfg.FPS <= 0 {
		errs = append(errs, "fps must be positive")
	}
	if cfg.Width <= 0 {
		errs = append(errs, "width must be positive")
	}
	if cfg.Height <= 0 {
		errs = append(errs, "height must be positive")
	}
	if cfg.Soup.ValidationEnabled {
		if cfg.Soup.MinConfidence <= 0 || cfg.Soup.MinConfidence > 1 {
			errs = append(errs, "soup.min_confidence must be in (0, 1]")
		}
		if cfg.Soup.BorderlineThreshold <= 0 || cfg.Soup.BorderlineThreshold >= cfg.Soup.MinConfidence {
			errs = append(errs, "soup.borderline_threshold must be positive and less than min_confidence")
		}
	}
	if cfg.Music.Enabled && cfg.Music.DurationSeconds <= 0 {
		errs = append(errs, "music.duration_seconds must be positive when music is enabled")
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
