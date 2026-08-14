// Package render orchestrates slide-frame rendering and audio mixing with
// restart-safe caching via content fingerprints. No external media executables
// are required; tests use LocalRenderer and LocalMixer.
//
// # Cache Protocol
//
// Execute writes outputs to outputDir in the following order:
//  1. Render frames into outputDir/frames/
//  2. Mix audio into outputDir/mixed.m4a (computes SHA-256 for the manifest)
//  3. Write render.json manifest (atomic rename)
//  4. Write render.done completion marker (atomic rename)
//
// A cached result is only served when render.json exists with a matching
// fingerprint AND render.done is present. The two-file protocol guarantees that
// a crash between steps 3 and 4 causes the next run to re-execute rather than
// return a partial result.
//
// # Offline vs. production boundaries
//
// LocalRenderer and LocalMixer are for deterministic, executable-free offline
// use (tests and dry-run pipelines). They write JSON metadata files and text
// manifests instead of real video frames or mixed audio. For production use,
// implement FrameRenderer and AudioMixer with Remotion and FFmpeg respectively
// (see remotion.go and ffmpeg.go).
package render

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/persistence"
)

const renderManifestSchemaVersion = 1

// ────────── Configuration ──────────

// FrameConfig describes the video output geometry and frame rate.
type FrameConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	FPS    int `json:"fps"`
}

// DefaultFrameConfig returns sensible HD defaults.
func DefaultFrameConfig() FrameConfig {
	return FrameConfig{Width: 1920, Height: 1080, FPS: 30}
}

// ────────── Plan ──────────

// Plan is the complete, immutable description of a render job. A Plan is
// derived deterministically from config and content so that two Plans with the
// same fingerprint are guaranteed to produce identical output.
type Plan struct {
	// Fingerprint is the sha256 hex of all plan inputs. Two identical Plans
	// produce the same fingerprint; any change produces a different one.
	Fingerprint string
	Config      FrameConfig
	Slides      []domain.Slide
	// AudioPaths is the ordered list of per-chunk audio files to mix.
	AudioPaths []string
	// ContentHash is the CombinedHash from the slides package, embedded for
	// lineage tracing.
	ContentHash string
}

// BuildPlan constructs a deterministic render Plan from slides, config, and
// ordered audio paths.
func BuildPlan(slides []domain.Slide, config FrameConfig, audioPaths []string, contentHash string) Plan {
	fp := planFingerprint(slides, config, audioPaths, contentHash)
	return Plan{
		Fingerprint: fp,
		Config:      config,
		Slides:      slides,
		AudioPaths:  audioPaths,
		ContentHash: contentHash,
	}
}

func planFingerprint(slides []domain.Slide, config FrameConfig, audioPaths []string, contentHash string) string {
	h := sha256.New()
	// Config
	fmt.Fprintf(h, "w=%d,h=%d,fps=%d\n", config.Width, config.Height, config.FPS)
	// Slides – include number, title and duration (body content is in contentHash)
	for _, s := range slides {
		fmt.Fprintf(h, "slide=%d;title=%s;dur=%.3f\n", s.Number, s.Title, s.DurationSeconds)
	}
	// Audio paths – order is significant: different orderings produce different mixes.
	for _, p := range audioPaths {
		fmt.Fprintf(h, "audio=%s\n", p)
	}
	// Content hash from slides package
	fmt.Fprintf(h, "content=%s\n", contentHash)
	return hex.EncodeToString(h.Sum(nil))
}

// ────────── Result & manifest ──────────

// Result is the outcome of a completed render job.
type Result struct {
	// Fingerprint matches Plan.Fingerprint.
	Fingerprint string `json:"fingerprint"`
	// FrameDir is the directory where frame data was written.
	FrameDir string `json:"frame_dir"`
	// MixedAudio is the path of the mixed audio file.
	MixedAudio string `json:"mixed_audio"`
	// FrameCount is the number of frames written.
	FrameCount int `json:"frame_count"`
	// MixedAudioSHA256 is the SHA-256 hex of the mixed audio file, enabling
	// downstream corruption detection.
	MixedAudioSHA256 string `json:"mixed_audio_sha256,omitempty"`
	// FromCache is true when the result was loaded from a prior run's manifest.
	FromCache bool `json:"from_cache"`
}

// renderManifest is written alongside render outputs for restart detection.
type renderManifest struct {
	SchemaVersion    int       `json:"schema_version"`
	Fingerprint      string    `json:"fingerprint"`
	FrameDir         string    `json:"frame_dir"`
	MixedAudio       string    `json:"mixed_audio"`
	FrameCount       int       `json:"frame_count"`
	MixedAudioSHA256 string    `json:"mixed_audio_sha256,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func manifestPath(outputDir string) string {
	return filepath.Join(outputDir, "render.json")
}

// doneMarkerPath returns the path of the atomic completion marker written after
// a successful render. Its presence alongside a valid render.json guarantees
// the render completed without interruption.
func doneMarkerPath(outputDir string) string {
	return filepath.Join(outputDir, "render.done")
}

// ────────── Renderer / Mixer interfaces ──────────

// FrameRenderer renders a single slide's frames into frameDir.
// start is the first frame index; count is the number of frames to render.
type FrameRenderer interface {
	RenderSlide(ctx context.Context, slide domain.Slide, frameStart, frameCount int, frameDir string) error
}

// AudioMixer mixes a list of input audio paths into a single output file.
type AudioMixer interface {
	MixAudio(ctx context.Context, inputPaths []string, outputPath string) error
}

// ────────── Execute ──────────

// Execute runs a render plan, writing frames to outputDir/frames/ and mixed
// audio to outputDir/mixed.m4a. When a render.json manifest with a matching
// fingerprint AND a render.done completion marker both exist in outputDir the
// call returns immediately (restart-safe).
//
// Progress is not retried on partial runs; delete outputDir to force a full
// re-render.
func Execute(ctx context.Context, renderer FrameRenderer, mixer AudioMixer, plan Plan, outputDir string) (Result, error) {
	// Restart-safe: check for valid cached result (manifest + done marker).
	if result, ok := loadCachedResult(outputDir, plan.Fingerprint); ok {
		result.FromCache = true
		return result, nil
	}

	frameDir := filepath.Join(outputDir, "frames")
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("render: mkdir frames: %w", err)
	}

	// Render each slide's frames in sequence.
	totalFrames := 0
	frameStart := 0
	for _, slide := range plan.Slides {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		dur := slide.DurationSeconds
		if dur <= 0 {
			dur = 1.0
		}
		frameCount := int(dur*float64(plan.Config.FPS) + 0.5)
		if frameCount < 1 {
			frameCount = 1
		}
		if err := renderer.RenderSlide(ctx, slide, frameStart, frameCount, frameDir); err != nil {
			return Result{}, fmt.Errorf("render: slide %d: %w", slide.Number, err)
		}
		frameStart += frameCount
		totalFrames += frameCount
	}

	// Mix audio.
	mixedAudio := filepath.Join(outputDir, "mixed.m4a")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("render: mkdir output: %w", err)
	}
	if len(plan.AudioPaths) > 0 {
		if err := mixer.MixAudio(ctx, plan.AudioPaths, mixedAudio); err != nil {
			return Result{}, fmt.Errorf("render: mix audio: %w", err)
		}
	} else {
		// No audio: write an empty placeholder so Validate succeeds.
		if err := os.WriteFile(mixedAudio, []byte{}, 0o644); err != nil {
			return Result{}, fmt.Errorf("render: write empty audio: %w", err)
		}
	}

	// Compute mixed audio checksum for integrity chain.
	mixedAudioSHA, err := fileSHA256(mixedAudio)
	if err != nil {
		return Result{}, fmt.Errorf("render: checksum mixed audio: %w", err)
	}

	result := Result{
		Fingerprint:      plan.Fingerprint,
		FrameDir:         frameDir,
		MixedAudio:       mixedAudio,
		FrameCount:       totalFrames,
		MixedAudioSHA256: mixedAudioSHA,
	}

	// Step 1: write manifest atomically.
	if err := persistManifest(outputDir, result); err != nil {
		return Result{}, err
	}
	// Step 2: write completion marker atomically. If this step fails the next
	// run will re-render (marker absent → cache miss).
	if err := writeCompletionMarker(outputDir); err != nil {
		return Result{}, err
	}
	return result, nil
}

// Validate checks that a Result's outputs are present on disk.
func Validate(result Result) error {
	if _, err := os.Stat(result.FrameDir); err != nil {
		return fmt.Errorf("render: frame dir missing: %w", err)
	}
	if _, err := os.Stat(result.MixedAudio); err != nil {
		return fmt.Errorf("render: mixed audio missing: %w", err)
	}
	return nil
}

// ────────── internal persistence ──────────

func loadCachedResult(outputDir, fingerprint string) (Result, bool) {
	// Require both manifest and completion marker.
	if _, err := os.Stat(doneMarkerPath(outputDir)); err != nil {
		return Result{}, false
	}
	var m renderManifest
	if err := persistence.ReadJSON(manifestPath(outputDir), &m); err != nil {
		return Result{}, false
	}
	if m.SchemaVersion != renderManifestSchemaVersion {
		return Result{}, false
	}
	if m.Fingerprint != fingerprint {
		return Result{}, false
	}
	return Result{
		Fingerprint:      m.Fingerprint,
		FrameDir:         m.FrameDir,
		MixedAudio:       m.MixedAudio,
		FrameCount:       m.FrameCount,
		MixedAudioSHA256: m.MixedAudioSHA256,
	}, true
}

func persistManifest(outputDir string, result Result) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("render: mkdir for manifest: %w", err)
	}
	m := renderManifest{
		SchemaVersion:    renderManifestSchemaVersion,
		Fingerprint:      result.Fingerprint,
		FrameDir:         result.FrameDir,
		MixedAudio:       result.MixedAudio,
		FrameCount:       result.FrameCount,
		MixedAudioSHA256: result.MixedAudioSHA256,
		CreatedAt:        time.Now().UTC(),
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("render: marshal manifest: %w", err)
	}
	return writeFileAtomic(manifestPath(outputDir), append(data, '\n'))
}

// writeCompletionMarker atomically writes the render.done sentinel file.
func writeCompletionMarker(outputDir string) error {
	return writeFileAtomic(doneMarkerPath(outputDir), []byte("done\n"))
}

// writeFileAtomic writes data to path via a temp-file rename.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-render-")
	if err != nil {
		return fmt.Errorf("render: create temp for %s: %w", filepath.Base(path), err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("render: write temp %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("render: sync temp %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("render: close temp %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("render: rename to %s: %w", path, err)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ────────── manifest compilation helper ──────────

// CompileManifest builds a summary string for human-readable audit logs from a
// Result. It lists fingerprint, frame count, and output paths.
func CompileManifest(result Result) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "fingerprint: %s\n", result.Fingerprint)
	fmt.Fprintf(&sb, "frames: %d\n", result.FrameCount)
	fmt.Fprintf(&sb, "frame_dir: %s\n", result.FrameDir)
	fmt.Fprintf(&sb, "mixed_audio: %s\n", result.MixedAudio)
	if result.MixedAudioSHA256 != "" {
		fmt.Fprintf(&sb, "mixed_audio_sha256: %s\n", result.MixedAudioSHA256)
	}
	if result.FromCache {
		sb.WriteString("source: cache\n")
	} else {
		sb.WriteString("source: rendered\n")
	}
	return sb.String()
}
