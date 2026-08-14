package render_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/render"
)

// ────────── helpers ──────────

func slide(num int, title string, dur float64) domain.Slide {
	return domain.Slide{Number: num, Title: title, DurationSeconds: dur}
}

func defaultSlides() []domain.Slide {
	return []domain.Slide{
		slide(1, "Intro", 2.0),
		slide(2, "Body", 3.0),
	}
}

// ────────── FrameConfig ──────────

func TestDefaultFrameConfig(t *testing.T) {
	cfg := render.DefaultFrameConfig()
	if cfg.Width != 1920 || cfg.Height != 1080 || cfg.FPS != 30 {
		t.Errorf("unexpected DefaultFrameConfig: %+v", cfg)
	}
}

// ────────── BuildPlan ──────────

func TestBuildPlan_Fingerprint_NotEmpty(t *testing.T) {
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "hash123")
	if plan.Fingerprint == "" {
		t.Error("Fingerprint must not be empty")
	}
	if len(plan.Fingerprint) != 64 {
		t.Errorf("Fingerprint len = %d, want 64 (sha256 hex)", len(plan.Fingerprint))
	}
}

func TestBuildPlan_Deterministic(t *testing.T) {
	slides := defaultSlides()
	cfg := render.DefaultFrameConfig()
	p1 := render.BuildPlan(slides, cfg, nil, "ch")
	p2 := render.BuildPlan(slides, cfg, nil, "ch")
	if p1.Fingerprint != p2.Fingerprint {
		t.Error("BuildPlan must be deterministic: same inputs must produce same fingerprint")
	}
}

func TestBuildPlan_FingerprintChanges_DifferentConfig(t *testing.T) {
	slides := defaultSlides()
	cfg1 := render.DefaultFrameConfig()
	cfg2 := render.FrameConfig{Width: 1280, Height: 720, FPS: 25}
	p1 := render.BuildPlan(slides, cfg1, nil, "ch")
	p2 := render.BuildPlan(slides, cfg2, nil, "ch")
	if p1.Fingerprint == p2.Fingerprint {
		t.Error("different config must produce different fingerprint")
	}
}

func TestBuildPlan_FingerprintChanges_DifferentContentHash(t *testing.T) {
	slides := defaultSlides()
	cfg := render.DefaultFrameConfig()
	p1 := render.BuildPlan(slides, cfg, nil, "hash-aaa")
	p2 := render.BuildPlan(slides, cfg, nil, "hash-bbb")
	if p1.Fingerprint == p2.Fingerprint {
		t.Error("different contentHash must produce different fingerprint")
	}
}

func TestBuildPlan_FingerprintChanges_DifferentAudio(t *testing.T) {
	slides := defaultSlides()
	cfg := render.DefaultFrameConfig()
	p1 := render.BuildPlan(slides, cfg, []string{"a.mp3"}, "ch")
	p2 := render.BuildPlan(slides, cfg, []string{"b.mp3"}, "ch")
	if p1.Fingerprint == p2.Fingerprint {
		t.Error("different audio paths must produce different fingerprint")
	}
}

func TestBuildPlan_PreservesFields(t *testing.T) {
	slides := defaultSlides()
	cfg := render.DefaultFrameConfig()
	paths := []string{"audio1.mp3", "audio2.mp3"}
	plan := render.BuildPlan(slides, cfg, paths, "content-hash")

	if plan.Config != cfg {
		t.Errorf("Config not preserved: %+v", plan.Config)
	}
	if plan.ContentHash != "content-hash" {
		t.Errorf("ContentHash = %q, want content-hash", plan.ContentHash)
	}
	if len(plan.Slides) != 2 {
		t.Errorf("Slides count = %d, want 2", len(plan.Slides))
	}
	if len(plan.AudioPaths) != 2 {
		t.Errorf("AudioPaths count = %d, want 2", len(plan.AudioPaths))
	}
}

// ────────── LocalRenderer ──────────

func TestLocalRenderer_WritesFrameFiles(t *testing.T) {
	dir := t.TempDir()
	r := &render.LocalRenderer{}
	s := slide(1, "Intro", 0)
	if err := r.RenderSlide(context.Background(), s, 0, 3, dir); err != nil {
		t.Fatalf("RenderSlide: %v", err)
	}
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, fmt.Sprintf("%08d.json", i))
		if _, err := os.Stat(name); err != nil {
			t.Errorf("frame file %s not written", name)
		}
	}
}

func TestLocalRenderer_FrameFileContainsSlideInfo(t *testing.T) {
	dir := t.TempDir()
	r := &render.LocalRenderer{}
	s := slide(2, "My Slide", 0)
	r.RenderSlide(context.Background(), s, 0, 1, dir)
	data, err := os.ReadFile(filepath.Join(dir, "00000000.json"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `"slide":2`) {
		t.Errorf("frame file must contain slide number; got: %s", content)
	}
	if !strings.Contains(content, "My Slide") {
		t.Errorf("frame file must contain slide title; got: %s", content)
	}
}

func TestLocalRenderer_IncrementsCallCount(t *testing.T) {
	r := &render.LocalRenderer{}
	dir := t.TempDir()
	r.RenderSlide(context.Background(), slide(1, "a", 0), 0, 1, dir)
	r.RenderSlide(context.Background(), slide(2, "b", 0), 1, 1, dir)
	if r.CallCount != 2 {
		t.Errorf("CallCount = %d, want 2", r.CallCount)
	}
}

func TestLocalRenderer_ForceErr(t *testing.T) {
	r := &render.LocalRenderer{ForceErr: fmt.Errorf("forced")}
	err := r.RenderSlide(context.Background(), slide(1, "a", 0), 0, 1, t.TempDir())
	if err == nil {
		t.Error("expected error from ForceErr")
	}
}

// ────────── LocalMixer ──────────

func TestLocalMixer_WritesOutputFile(t *testing.T) {
	dir := t.TempDir()
	m := &render.LocalMixer{}
	out := filepath.Join(dir, "mixed.m4a")
	if err := m.MixAudio(context.Background(), []string{"a.mp3", "b.mp3"}, out); err != nil {
		t.Fatalf("MixAudio: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("output file not written: %v", err)
	}
}

func TestLocalMixer_OutputContainsPaths(t *testing.T) {
	dir := t.TempDir()
	m := &render.LocalMixer{}
	out := filepath.Join(dir, "mixed.m4a")
	m.MixAudio(context.Background(), []string{"track1.mp3", "track2.mp3"}, out)
	data, _ := os.ReadFile(out)
	content := string(data)
	if !strings.Contains(content, "track1.mp3") || !strings.Contains(content, "track2.mp3") {
		t.Errorf("mixed output must list input paths; got: %s", content)
	}
}

func TestLocalMixer_ForceErr(t *testing.T) {
	m := &render.LocalMixer{ForceErr: fmt.Errorf("forced")}
	err := m.MixAudio(context.Background(), nil, filepath.Join(t.TempDir(), "out.m4a"))
	if err == nil {
		t.Error("expected error from ForceErr")
	}
}

// ────────── Execute ──────────

func TestExecute_WritesFramesAndMixedAudio(t *testing.T) {
	dir := t.TempDir()
	slides := defaultSlides() // 2s + 3s at 30fps = 60+90 = 150 frames
	plan := render.BuildPlan(slides, render.DefaultFrameConfig(), nil, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	result, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.FrameCount == 0 {
		t.Error("FrameCount must be > 0")
	}
	if result.FrameDir == "" {
		t.Error("FrameDir must not be empty")
	}
	if result.MixedAudio == "" {
		t.Error("MixedAudio must not be empty")
	}
	if result.Fingerprint != plan.Fingerprint {
		t.Errorf("Result.Fingerprint = %q, want %q", result.Fingerprint, plan.Fingerprint)
	}
}

func TestExecute_FrameCountMatchesDuration(t *testing.T) {
	dir := t.TempDir()
	slides := []domain.Slide{slide(1, "A", 2.0)} // 2s * 30fps = 60 frames
	cfg := render.DefaultFrameConfig()
	plan := render.BuildPlan(slides, cfg, nil, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	result, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.FrameCount != 60 {
		t.Errorf("FrameCount = %d, want 60 (2s * 30fps)", result.FrameCount)
	}
}

func TestExecute_RestartSafe_SkipsOnMatchingFingerprint(t *testing.T) {
	dir := t.TempDir()
	slides := defaultSlides()
	plan := render.BuildPlan(slides, render.DefaultFrameConfig(), nil, "ch")

	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	// First execution.
	_, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	calls1 := r.CallCount

	// Second execution with same plan: must use cache.
	result2, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result2.FromCache {
		t.Error("second Execute must return FromCache=true")
	}
	if r.CallCount != calls1 {
		t.Errorf("renderer called again on cache hit: calls went from %d to %d", calls1, r.CallCount)
	}
}

func TestExecute_RestartSafe_RerenderOnFingerprintChange(t *testing.T) {
	dir := t.TempDir()
	slides := defaultSlides()
	plan1 := render.BuildPlan(slides, render.DefaultFrameConfig(), nil, "hash-v1")
	plan2 := render.BuildPlan(slides, render.DefaultFrameConfig(), nil, "hash-v2")

	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	render.Execute(context.Background(), r, m, plan1, dir)
	calls1 := r.CallCount

	result2, err := render.Execute(context.Background(), r, m, plan2, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result2.FromCache {
		t.Error("different fingerprint must not return FromCache=true")
	}
	if r.CallCount == calls1 {
		t.Error("renderer must be called again when fingerprint changes")
	}
}

func TestExecute_RendererError_Propagates(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	r := &render.LocalRenderer{ForceErr: fmt.Errorf("render failed")}
	m := &render.LocalMixer{}

	_, err := render.Execute(context.Background(), r, m, plan, dir)
	if err == nil {
		t.Error("expected error when renderer fails")
	}
}

func TestExecute_MixerError_Propagates(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), []string{"a.mp3"}, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{ForceErr: fmt.Errorf("mix failed")}

	_, err := render.Execute(context.Background(), r, m, plan, dir)
	if err == nil {
		t.Error("expected error when mixer fails")
	}
}

// ────────── Validate ──────────

func TestValidate_HappyPath(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	result, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := render.Validate(result); err != nil {
		t.Errorf("Validate failed on good result: %v", err)
	}
}

func TestValidate_MissingFrameDir_ReturnsError(t *testing.T) {
	result := render.Result{
		FrameDir:   "/nonexistent/frames",
		MixedAudio: t.TempDir(), // points somewhere that exists
	}
	// create a temp file for MixedAudio so only FrameDir is missing
	tmp, _ := os.CreateTemp("", "audio*.m4a")
	tmp.Close()
	defer os.Remove(tmp.Name())
	result.MixedAudio = tmp.Name()

	if err := render.Validate(result); err == nil {
		t.Error("expected error for missing frame dir")
	}
}

func TestValidate_MissingMixedAudio_ReturnsError(t *testing.T) {
	result := render.Result{
		FrameDir:   t.TempDir(),
		MixedAudio: "/nonexistent/mixed.m4a",
	}
	if err := render.Validate(result); err == nil {
		t.Error("expected error for missing mixed audio")
	}
}

// ────────── CompileManifest ──────────

func TestCompileManifest_ContainsFingerprint(t *testing.T) {
	result := render.Result{Fingerprint: "abc123", FrameDir: "/frames", MixedAudio: "/mix.m4a", FrameCount: 42}
	manifest := render.CompileManifest(result)
	if !strings.Contains(manifest, "abc123") {
		t.Errorf("CompileManifest must contain fingerprint; got: %s", manifest)
	}
}

func TestCompileManifest_ContainsFrameCount(t *testing.T) {
	result := render.Result{Fingerprint: "fp", FrameDir: "/frames", MixedAudio: "/mix.m4a", FrameCount: 99}
	manifest := render.CompileManifest(result)
	if !strings.Contains(manifest, "99") {
		t.Errorf("CompileManifest must contain frame count; got: %s", manifest)
	}
}

func TestCompileManifest_CacheSource(t *testing.T) {
	result := render.Result{FromCache: true}
	if !strings.Contains(render.CompileManifest(result), "cache") {
		t.Error("CompileManifest must indicate cache source when FromCache=true")
	}
	result.FromCache = false
	if !strings.Contains(render.CompileManifest(result), "rendered") {
		t.Error("CompileManifest must indicate rendered source when FromCache=false")
	}
}

// ────────── Completion marker ──────────

func TestExecute_WritesCompletionMarker(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	_, err := render.Execute(context.Background(), &render.LocalRenderer{}, &render.LocalMixer{}, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "render.done")); err != nil {
		t.Error("render.done completion marker must exist after successful Execute")
	}
}

func TestExecute_MissingCompletionMarker_RerenderOnNextCall(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	// First render.
	_, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	calls1 := r.CallCount

	// Remove completion marker to simulate crash after manifest write.
	os.Remove(filepath.Join(dir, "render.done"))

	// Next call must not use cache (completion marker absent).
	result2, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result2.FromCache {
		t.Error("FromCache must be false when completion marker is missing")
	}
	if r.CallCount == calls1 {
		t.Error("renderer must be called again when completion marker is missing")
	}
}

func TestExecute_ResultHasMixedAudioSHA256(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	result, err := render.Execute(context.Background(), &render.LocalRenderer{}, &render.LocalMixer{}, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.MixedAudioSHA256 == "" {
		t.Error("Result.MixedAudioSHA256 must not be empty after successful render")
	}
	if len(result.MixedAudioSHA256) != 64 {
		t.Errorf("MixedAudioSHA256 len = %d, want 64 (sha256 hex)", len(result.MixedAudioSHA256))
	}
}

func TestExecute_CacheHit_PreservesMixedAudioSHA256(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	res1, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.FromCache {
		t.Fatal("second call must be cache hit")
	}
	if res2.MixedAudioSHA256 != res1.MixedAudioSHA256 {
		t.Errorf("MixedAudioSHA256 mismatch: got %q, want %q", res2.MixedAudioSHA256, res1.MixedAudioSHA256)
	}
}

func TestExecute_CorruptManifest_Rerenders(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	_, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	calls1 := r.CallCount

	// Corrupt the manifest.
	os.WriteFile(filepath.Join(dir, "render.json"), []byte("not valid json"), 0o644)

	result2, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result2.FromCache {
		t.Error("corrupt manifest must not be returned as cache hit")
	}
	if r.CallCount == calls1 {
		t.Error("renderer must be called when manifest is corrupt")
	}
}

func TestExecute_StaleSchemaVersion_Rerenders(t *testing.T) {
	dir := t.TempDir()
	plan := render.BuildPlan(defaultSlides(), render.DefaultFrameConfig(), nil, "ch")
	r := &render.LocalRenderer{}
	m := &render.LocalMixer{}

	_, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	calls1 := r.CallCount

	// Write a manifest with schema_version 0 (stale).
	raw, _ := os.ReadFile(filepath.Join(dir, "render.json"))
	stale := strings.Replace(string(raw), `"schema_version": 1`, `"schema_version": 0`, 1)
	os.WriteFile(filepath.Join(dir, "render.json"), []byte(stale), 0o644)

	result2, err := render.Execute(context.Background(), r, m, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result2.FromCache {
		t.Error("stale schema version must not be returned as cache hit")
	}
	if r.CallCount == calls1 {
		t.Error("renderer must be called when manifest schema version is stale")
	}
}

func TestCompileManifest_ContainsSHA256(t *testing.T) {
	result := render.Result{
		Fingerprint:      "fp",
		FrameDir:         "/frames",
		MixedAudio:       "/mix.m4a",
		FrameCount:       10,
		MixedAudioSHA256: "abc123",
	}
	m := render.CompileManifest(result)
	if !strings.Contains(m, "abc123") {
		t.Errorf("CompileManifest must contain MixedAudioSHA256; got: %s", m)
	}
}
