package tts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nebula/course-video-pipeline/internal/narration"
	"github.com/nebula/course-video-pipeline/internal/providers"
	"github.com/nebula/course-video-pipeline/internal/tts"
)

func makeChunk(id, text string) narration.Chunk {
	return narration.Chunk{ID: id, Text: text, SlideNumber: 1, CharCount: len(text)}
}

// ────────── FakeProvider ──────────

func TestFakeProvider_Synthesize_WritesBytes(t *testing.T) {
	fp := &tts.FakeProvider{}
	var buf byteWriter
	u, err := fp.Synthesize(context.Background(), "hello", "v1", &buf)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if len(buf.data) == 0 {
		t.Error("expected non-empty audio bytes")
	}
	if u.Provider != "fake" {
		t.Errorf("Usage.Provider = %q, want fake", u.Provider)
	}
	if u.Characters != len("hello") {
		t.Errorf("Usage.Characters = %d, want %d", u.Characters, len("hello"))
	}
}

func TestFakeProvider_Synthesize_Deterministic(t *testing.T) {
	fp := &tts.FakeProvider{}
	var b1, b2 byteWriter
	fp.Synthesize(context.Background(), "test text", "v1", &b1)
	fp.Synthesize(context.Background(), "test text", "v1", &b2)
	if string(b1.data) != string(b2.data) {
		t.Error("FakeProvider must be deterministic: same text must produce same bytes")
	}
}

func TestFakeProvider_Synthesize_DifferentText_DifferentBytes(t *testing.T) {
	fp := &tts.FakeProvider{}
	var b1, b2 byteWriter
	fp.Synthesize(context.Background(), "text one", "v1", &b1)
	fp.Synthesize(context.Background(), "text two", "v1", &b2)
	if string(b1.data) == string(b2.data) {
		t.Error("different texts must produce different bytes")
	}
}

func TestFakeProvider_Synthesize_CountsCall(t *testing.T) {
	fp := &tts.FakeProvider{}
	fp.Synthesize(context.Background(), "a", "v1", &byteWriter{})
	fp.Synthesize(context.Background(), "b", "v1", &byteWriter{})
	if fp.CallCount != 2 {
		t.Errorf("CallCount = %d, want 2", fp.CallCount)
	}
}

func TestFakeProvider_ForceErr_ReturnsErr(t *testing.T) {
	fp := &tts.FakeProvider{ForceErr: fmt.Errorf("forced")}
	_, err := fp.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if err == nil {
		t.Error("expected error from ForceErr")
	}
}

// ────────── IsTransient / TransientError ──────────

func TestIsTransient_WrappedError(t *testing.T) {
	err := fmt.Errorf("wrap: %w", tts.ErrTransient)
	if !tts.IsTransient(err) {
		t.Error("IsTransient must return true for wrapped ErrTransient")
	}
}

func TestIsTransient_OtherError(t *testing.T) {
	if tts.IsTransient(fmt.Errorf("permanent")) {
		t.Error("IsTransient must return false for non-transient error")
	}
}

func TestTransientError_IsTransient(t *testing.T) {
	err := tts.TransientError("rate limited")
	if !tts.IsTransient(err) {
		t.Error("TransientError result must satisfy IsTransient")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error message = %q, want to contain 'rate limited'", err.Error())
	}
}

// ────────── Synthesize – cache behaviour ──────────

func TestSynthesize_CacheMiss_WritesFileAndSidecar(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "v1"}

	res, rec, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if res.FromCache {
		t.Error("expected FromCache=false on first call")
	}
	if fp.CallCount != 1 {
		t.Errorf("provider called %d times, want 1", fp.CallCount)
	}
	if rec == nil {
		t.Error("usage record must not be nil on cache miss")
	}
	if rec.Characters != len(chunk.Text) {
		t.Errorf("usage Characters = %d, want %d", rec.Characters, len(chunk.Text))
	}
	if _, err := os.Stat(res.AudioPath); err != nil {
		t.Errorf("audio file not written: %v", err)
	}
	// Sidecar must also be written.
	scPath := tts.SidecarPath(res.AudioPath)
	if _, err := os.Stat(scPath); err != nil {
		t.Errorf("sidecar file not written: %v", err)
	}
	if res.Sidecar == nil {
		t.Fatal("AudioResult.Sidecar must not be nil after cache miss synthesis")
	}
	if res.Sidecar.ChunkID != chunk.ID {
		t.Errorf("sidecar ChunkID = %q, want %q", res.Sidecar.ChunkID, chunk.ID)
	}
	if res.Sidecar.AudioSHA256 == "" {
		t.Error("sidecar AudioSHA256 must not be empty")
	}
}

func TestSynthesize_CacheHit_SkipsProvider(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "v1"}

	// First call populates cache.
	_, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatalf("first Synthesize: %v", err)
	}
	callsAfterFirst := fp.CallCount

	// Second call must hit cache.
	res, rec, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatalf("second Synthesize: %v", err)
	}
	if !res.FromCache {
		t.Error("expected FromCache=true on second call")
	}
	if fp.CallCount != callsAfterFirst {
		t.Errorf("provider called %d times after first call; should be 0 extra on cache hit", fp.CallCount-callsAfterFirst)
	}
	if rec != nil {
		t.Error("usage record must be nil on cache hit")
	}
	if res.Sidecar == nil {
		t.Error("AudioResult.Sidecar must not be nil on cache hit")
	}
}

func TestSynthesize_SecondCall_FromCache(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "v1"}

	tts.Synthesize(context.Background(), fp, chunk, opts)
	res, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatalf("second Synthesize: %v", err)
	}
	if !res.FromCache {
		t.Error("second call must hit cache")
	}
	if fp.CallCount != 1 {
		t.Errorf("provider called %d times total, want 1", fp.CallCount)
	}
}

func TestSynthesize_NoCache_DoesNotWriteFile(t *testing.T) {
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{VoiceID: "v1"} // no CacheDir

	res, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if res.AudioPath != "" {
		t.Errorf("AudioPath = %q, want empty when no CacheDir", res.AudioPath)
	}
	if res.Sidecar != nil {
		t.Error("Sidecar must be nil when CacheDir is empty")
	}
}

func TestSynthesize_UsageRecord_Fields(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "voice-123"}

	_, rec, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Voice != "voice-123" {
		t.Errorf("UsageRecord.Voice = %q, want voice-123", rec.Voice)
	}
	if rec.Provider != "fake" {
		t.Errorf("UsageRecord.Provider = %q, want fake", rec.Provider)
	}
	if rec.At.IsZero() {
		t.Error("UsageRecord.At must not be zero")
	}
}

// ────────── Sidecar validation ──────────

func TestSynthesize_StaleSchemaVersion_Resynthesizes(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "v1"}

	// Populate valid cache.
	res, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := fp.CallCount

	// Corrupt sidecar: wrong schema version.
	scPath := tts.SidecarPath(res.AudioPath)
	raw, _ := os.ReadFile(scPath)
	var sc map[string]interface{}
	json.Unmarshal(raw, &sc)
	sc["schema_version"] = 999
	data, _ := json.Marshal(sc)
	os.WriteFile(scPath, data, 0o644)

	// Next call must re-synthesize.
	_, _, err = tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	if fp.CallCount == callsAfterFirst {
		t.Error("provider must be called again when sidecar schema version is stale")
	}
}

func TestSynthesize_CorruptAudio_Resynthesizes(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "v1"}

	res, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := fp.CallCount

	// Corrupt audio file (different bytes → SHA-256 mismatch).
	os.WriteFile(res.AudioPath, []byte("corrupt"), 0o644)

	_, _, err = tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	if fp.CallCount == callsAfterFirst {
		t.Error("provider must be called again when audio checksum mismatches sidecar")
	}
}

func TestSynthesize_AudioWithoutSidecar_Resynthesizes(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "v1"}

	res, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := fp.CallCount

	// Remove sidecar; leave audio.
	os.Remove(tts.SidecarPath(res.AudioPath))

	_, _, err = tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	if fp.CallCount == callsAfterFirst {
		t.Error("provider must be called when sidecar is missing")
	}
}

func TestSynthesize_VoiceChange_Resynthesizes(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")

	// Cache with voice v1.
	tts.Synthesize(context.Background(), fp, chunk, tts.Options{CacheDir: dir, VoiceID: "v1"})
	callsAfterFirst := fp.CallCount

	// Different voice ID: idempotency key changes → cache miss.
	tts.Synthesize(context.Background(), fp, chunk, tts.Options{CacheDir: dir, VoiceID: "v2"})
	if fp.CallCount == callsAfterFirst {
		t.Error("changing voice ID must invalidate cache")
	}
}

func TestSynthesize_SidecarContainsIdempotencyKey(t *testing.T) {
	dir := t.TempDir()
	fp := &tts.FakeProvider{}
	chunk := makeChunk("abc123456789abcd", "Hello world.")
	opts := tts.Options{CacheDir: dir, VoiceID: "v1"}

	res, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sidecar.IdempotencyKey == "" {
		t.Error("sidecar IdempotencyKey must not be empty")
	}
	if res.Sidecar.TextFingerprint == "" {
		t.Error("sidecar TextFingerprint must not be empty")
	}
}

// ────────── Synthesize – retry ──────────

func TestSynthesize_TransientRetry_Succeeds(t *testing.T) {
	fp := newFailingProvider(2, true)
	chunk := makeChunk("abc123456789abcd", "Hello.")
	opts := tts.Options{MaxRetries: 3, VoiceID: "v1"}

	_, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Errorf("expected success after transient failures, got: %v", err)
	}
	if fp.calls < 3 {
		t.Errorf("expected ≥3 calls (2 failures + 1 success), got %d", fp.calls)
	}
}

func TestSynthesize_NonTransient_NoRetry(t *testing.T) {
	fp := newFailingProvider(5, false) // non-transient
	chunk := makeChunk("abc123456789abcd", "Hello.")
	opts := tts.Options{MaxRetries: 3, VoiceID: "v1"}

	_, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err == nil {
		t.Error("expected error on non-transient failure")
	}
	if fp.calls != 1 {
		t.Errorf("expected 1 call on non-transient error, got %d", fp.calls)
	}
}

func TestSynthesize_MaxRetries_Exhausted(t *testing.T) {
	fp := newFailingProvider(10, true)
	chunk := makeChunk("abc123456789abcd", "Hello.")
	opts := tts.Options{MaxRetries: 2, VoiceID: "v1"}

	_, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err == nil {
		t.Error("expected error after MaxRetries exhausted")
	}
	if fp.calls != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", fp.calls)
	}
}

func TestSynthesize_CancelledContext_AbortsRetry(t *testing.T) {
	fp := newFailingProvider(10, true)
	chunk := makeChunk("abc123456789abcd", "Hello.")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	opts := tts.Options{MaxRetries: 5, VoiceID: "v1"}
	_, _, err := tts.Synthesize(ctx, fp, chunk, opts)
	if err == nil {
		t.Error("expected error when context is cancelled")
	}
	if fp.calls > 1 {
		t.Errorf("cancelled context should stop retries early; got %d calls", fp.calls)
	}
}

func TestSynthesize_BackoffCalledOnRetry(t *testing.T) {
	fp := newFailingProvider(2, true)
	chunk := makeChunk("abc123456789abcd", "Hello.")
	backoffCalls := 0
	opts := tts.Options{
		MaxRetries: 3,
		VoiceID:    "v1",
		Backoff: func(retry int) time.Duration {
			backoffCalls++
			return 0 // no actual sleep in tests
		},
	}

	_, _, err := tts.Synthesize(context.Background(), fp, chunk, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backoffCalls == 0 {
		t.Error("Backoff must be called at least once on transient retry")
	}
}

// ────────── Helpers ──────────

type byteWriter struct{ data []byte }

func (b *byteWriter) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

// failingProvider implements providers.TTS and fails its first N calls.
type failingProvider struct {
	inner     providers.TTS
	failFirst int
	transient bool
	calls     int
}

func newFailingProvider(failFirst int, transient bool) *failingProvider {
	return &failingProvider{
		inner:     &tts.FakeProvider{},
		failFirst: failFirst,
		transient: transient,
	}
}

func (f *failingProvider) Synthesize(ctx context.Context, text, voiceID string, w io.Writer) (providers.Usage, error) {
	f.calls++
	if f.calls <= f.failFirst {
		if f.transient {
			return providers.Usage{}, fmt.Errorf("temporary network error: %w", tts.ErrTransient)
		}
		return providers.Usage{}, fmt.Errorf("permanent error")
	}
	return f.inner.Synthesize(ctx, text, voiceID, w)
}

// SidecarPath re-exports the unexported helper for white-box tests.
// We call tts.SidecarPath directly since it is exported.
var _ = filepath.Join // ensure filepath is used
