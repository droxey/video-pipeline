package pipeline_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/alignment"
	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/narration"
	"github.com/nebula/course-video-pipeline/internal/persistence"
	"github.com/nebula/course-video-pipeline/internal/pipeline"
	"github.com/nebula/course-video-pipeline/internal/render"
	"github.com/nebula/course-video-pipeline/internal/slides"
	"github.com/nebula/course-video-pipeline/internal/subtitles"
	"github.com/nebula/course-video-pipeline/internal/tts"
)

// ────────── fixtures ──────────

const e2eSlidesMD = `## Slide 1: Introduction

<!-- duration: 2.0 -->

Event-driven systems communicate through events rather than direct calls.

## Slide 2: Benefits

<!-- duration: 3.0 -->

Loose coupling and scalability are the primary advantages.
`

const e2eNotesMD = `## Slide 1

<!-- source: seg-0001 -->
<!-- approved-by: alice -->
<!-- approved-at: 2026-08-01T10:00:00Z -->

Welcome. [PAUSE:0.5] Today we explore event-driven architecture.

## Slide 2

<!-- source: seg-0002 -->
<!-- approved-by: alice -->
<!-- approved-at: 2026-08-02T09:00:00Z -->

The key benefits are loose coupling and scalability.
`

// ────────── End-to-end pipeline test ──────────

// TestE2E_FullPipeline runs the full pipeline from parse through render using
// only deterministic, in-process implementations (no network, no executables).
func TestE2E_FullPipeline(t *testing.T) {
	runDir := t.TempDir()
	ctx := context.Background()

	// ── Stage: Parse ─────────────────────────────────────────────
	parsedSlides, parseErrs := slides.ParseSlides(e2eSlidesMD)
	if len(parseErrs) != 0 {
		t.Fatalf("ParseSlides errors: %v", parseErrs)
	}
	if len(parsedSlides) != 2 {
		t.Fatalf("expected 2 slides, got %d", len(parsedSlides))
	}

	parsedNotes, noteErrs := slides.ParseNotes(e2eNotesMD)
	if len(noteErrs) != 0 {
		t.Fatalf("ParseNotes errors: %v", noteErrs)
	}

	matchResult := slides.Match(parsedSlides, parsedNotes, true)
	if len(matchResult.Errors) != 0 {
		t.Fatalf("Match errors: %v", matchResult.Errors)
	}
	if matchResult.CombinedHash == "" {
		t.Fatal("CombinedHash must not be empty")
	}

	// Convert ParsedSlides to domain.Slide for downstream stages.
	domainSlides := make([]domain.Slide, len(parsedSlides))
	for i, ps := range parsedSlides {
		domainSlides[i] = domain.Slide{
			Number:          ps.Number,
			Title:           ps.Title,
			DurationSeconds: ps.DurationSeconds,
			Body:            ps.Body,
		}
	}

	// ── Stage: Narration chunking ────────────────────────────────
	var inputs []narration.Input
	for _, ms := range matchResult.Slides {
		if ms.Note == nil {
			continue
		}
		inputs = append(inputs, narration.Input{
			SlideNumber: ms.Slide.Number,
			Text:        ms.Note.CleanText,
			SourceIDs:   ms.Note.SourceIDs,
		})
	}
	narOpts := narration.DefaultOptions()
	narResult := narration.Split(inputs, narOpts)
	if len(narResult.Errors) != 0 {
		t.Fatalf("narration split errors: %v", narResult.Errors)
	}
	if len(narResult.Chunks) == 0 {
		t.Fatal("expected at least one narration chunk")
	}
	schedule := narration.Schedule(narResult)
	if len(schedule) == 0 {
		t.Fatal("schedule must be non-empty")
	}

	// ── Stage: TTS synthesis ────────────────────────────────────
	ttsDir := filepath.Join(runDir, "tts")
	fakeTTS := &tts.FakeProvider{}
	var audioPaths []string
	for _, sc := range schedule {
		opts := tts.Options{CacheDir: ttsDir, VoiceID: "fake-voice"}
		res, rec, err := tts.Synthesize(ctx, fakeTTS, sc.Chunk, opts)
		if err != nil {
			t.Fatalf("Synthesize chunk %s: %v", sc.ID, err)
		}
		if res.AudioPath == "" {
			t.Errorf("AudioPath empty for chunk %s", sc.ID)
		}
		audioPaths = append(audioPaths, res.AudioPath)
		// Persist usage record (non-nil on cache miss).
		if rec != nil {
			if err := persistence.AppendUsage(runDir, rec); err != nil {
				t.Fatalf("AppendUsage: %v", err)
			}
		}
	}

	// ── Stage: Alignment ────────────────────────────────────────
	alignDir := filepath.Join(runDir, "alignment")
	aligner := &alignment.LocalAdapter{}
	for i, sc := range schedule {
		audioData, err := os.ReadFile(audioPaths[i])
		if err != nil {
			t.Fatalf("read audio %s: %v", audioPaths[i], err)
		}
		res, err := alignment.AlignBytes(ctx, aligner, sc.ID, audioData, sc.Text, alignDir)
		if err != nil {
			t.Fatalf("AlignBytes chunk %s: %v", sc.ID, err)
		}
		if len(res.Alignment.Words) == 0 {
			t.Errorf("alignment produced 0 words for chunk %s", sc.ID)
		}
		// Verify restart-safe: second call must use cache.
		res2, err := alignment.AlignBytes(ctx, aligner, sc.ID, audioData, sc.Text, alignDir)
		if err != nil {
			t.Fatalf("AlignBytes (second call) chunk %s: %v", sc.ID, err)
		}
		if len(res2.Alignment.Words) != len(res.Alignment.Words) {
			t.Errorf("reloaded alignment has %d words, want %d", len(res2.Alignment.Words), len(res.Alignment.Words))
		}
	}

	// ── Stage: Subtitles ───────────────────────────────────────
	// Load a persisted alignment result and build a subtitle track.
	firstChunk := schedule[0]
	alignResult, err := alignment.LoadResult(alignDir, firstChunk.ID)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	track := subtitles.BuildTrack(alignResult.Alignment, subtitles.BuildOptions{
		WordsPerCue: 5,
		Language:    "en",
		Format:      "vtt",
	})
	if len(track.Cues) == 0 {
		t.Error("expected at least one subtitle cue")
	}
	vtt := subtitles.FormatVTT(track)
	if !strings.HasPrefix(vtt, "WEBVTT\n") {
		t.Errorf("VTT output must start with WEBVTT header; got: %q", vtt[:min(20, len(vtt))])
	}
	srt := subtitles.FormatSRT(track)
	if !strings.Contains(srt, " --> ") {
		t.Error("SRT output must contain timestamp separator")
	}

	// ── Stage: Render ──────────────────────────────────────────
	renderDir := filepath.Join(runDir, "render")
	cfg := render.DefaultFrameConfig()
	plan := render.BuildPlan(domainSlides, cfg, audioPaths, matchResult.CombinedHash)

	renderer := &render.LocalRenderer{}
	mixer := &render.LocalMixer{}

	result, err := render.Execute(ctx, renderer, mixer, plan, renderDir)
	if err != nil {
		t.Fatalf("render.Execute: %v", err)
	}
	if result.FrameCount == 0 {
		t.Error("expected non-zero frame count")
	}
	if err := render.Validate(result); err != nil {
		t.Errorf("render.Validate: %v", err)
	}

	// Render must be restart-safe: second execution with same plan hits cache.
	result2, err := render.Execute(ctx, renderer, mixer, plan, renderDir)
	if err != nil {
		t.Fatalf("render.Execute (second): %v", err)
	}
	if !result2.FromCache {
		t.Error("second render.Execute must return FromCache=true")
	}

	// ── Stage: Manifest compilation ────────────────────────────
	manifest := render.CompileManifest(result)
	if !strings.Contains(manifest, plan.Fingerprint) {
		t.Errorf("manifest must contain plan fingerprint; got:\n%s", manifest)
	}

	// ── Stage: Pipeline manifest persistence ──────────────────
	pm := pipeline.NewManifest("run-e2e-001", "rec-001", matchResult.CombinedHash, "config-hash-abc")
	stages := []domain.Stage{
		domain.StageShortlist, domain.StageImport,
		domain.StageParse, domain.StageGenerate, domain.StageAlign, domain.StageRender,
	}
	for _, s := range stages {
		if err := pipeline.Transition(pm, s, domain.StageRunning); err != nil {
			t.Fatalf("Transition %s running: %v", s, err)
		}
		if err := pipeline.Transition(pm, s, domain.StageDone); err != nil {
			t.Fatalf("Transition %s done: %v", s, err)
		}
	}
	if err := persistence.SaveManifest(runDir, pm); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	loaded, err := persistence.LoadManifest(runDir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if loaded.RunID != "run-e2e-001" {
		t.Errorf("RunID = %q, want run-e2e-001", loaded.RunID)
	}
	if loaded.ContentHash != matchResult.CombinedHash {
		t.Errorf("ContentHash mismatch")
	}

	// ── Usage records ──────────────────────────────────────────
	usages, err := persistence.ReadUsage(runDir)
	if err != nil {
		t.Fatalf("ReadUsage: %v", err)
	}
	if len(usages) == 0 {
		t.Error("expected at least one usage record from TTS synthesis")
	}
}

// TestE2E_RestartResumesFromManifest verifies that a partial run can be resumed
// from a saved manifest without repeating completed stages.
func TestE2E_RestartResumesFromManifest(t *testing.T) {
	runDir := t.TempDir()

	contentHash := "content-abc"
	configHash := "config-xyz"
	pm := pipeline.NewManifest("run-resume-001", "rec-002", contentHash, configHash)

	// Simulate completing the first two stages.
	for _, s := range []domain.Stage{domain.StageShortlist, domain.StageImport, domain.StageParse, domain.StageGenerate} {
		pipeline.Transition(pm, s, domain.StageRunning)
		pipeline.Transition(pm, s, domain.StageDone)
	}

	if err := persistence.SaveManifest(runDir, pm); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	// Reload and check resumption point.
	loaded, err := persistence.LoadManifest(runDir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	resumeStage, ok := pipeline.ResumePoint(loaded)
	if !ok {
		t.Fatal("ResumePoint returned false; should find a pending stage")
	}
	if resumeStage != domain.StageAlign {
		t.Errorf("ResumePoint = %q, want align", resumeStage)
	}

	// Hash check: same hashes must pass.
	if err := pipeline.CanResume(loaded, contentHash, configHash); err != nil {
		t.Errorf("CanResume failed with matching hashes: %v", err)
	}

	// Mismatched content hash must fail.
	if err := pipeline.CanResume(loaded, "different-hash", configHash); err == nil {
		t.Error("CanResume must fail when content hash changes")
	}
}

// TestE2E_ParseFixtures runs parse+match on the testdata fixtures.
func TestE2E_ParseFixtures(t *testing.T) {
	slideData, err := os.ReadFile("../../testdata/fixtures/slides.md")
	if err != nil {
		t.Fatalf("read slides fixture: %v", err)
	}
	noteData, err := os.ReadFile("../../testdata/fixtures/speaker-notes.md")
	if err != nil {
		t.Fatalf("read notes fixture: %v", err)
	}

	parsedSlides, _ := slides.ParseSlides(string(slideData))
	parsedNotes, _ := slides.ParseNotes(string(noteData))
	result := slides.Match(parsedSlides, parsedNotes, false)

	if len(result.Slides) != 3 {
		t.Errorf("matched slides = %d, want 3", len(result.Slides))
	}
	if result.CombinedHash == "" {
		t.Error("CombinedHash must not be empty for fixture match")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
