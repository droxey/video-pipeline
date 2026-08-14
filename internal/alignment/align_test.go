package alignment_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/alignment"
	"github.com/nebula/course-video-pipeline/internal/providers"
)

// ────────── LocalAdapter ──────────

func TestLocalAdapter_ReturnsWords(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000) // 1 second at default bps
	al, err := a.Align(context.Background(), bytes.NewReader(audio), "hello world")
	if err != nil {
		t.Fatalf("Align: %v", err)
	}
	if len(al.Words) != 2 {
		t.Errorf("Words count = %d, want 2", len(al.Words))
	}
}

func TestLocalAdapter_WordText(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	al, _ := a.Align(context.Background(), bytes.NewReader(audio), "hello world")
	if len(al.Words) == 0 {
		t.Fatal("no words returned")
	}
	if al.Words[0].Word != "hello" {
		t.Errorf("Words[0].Word = %q, want hello", al.Words[0].Word)
	}
	if al.Words[1].Word != "world" {
		t.Errorf("Words[1].Word = %q, want world", al.Words[1].Word)
	}
}

func TestLocalAdapter_StartBeforeEnd(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	al, _ := a.Align(context.Background(), bytes.NewReader(audio), "one two three")
	for _, w := range al.Words {
		if w.Start >= w.End {
			t.Errorf("word %q: Start(%f) >= End(%f)", w.Word, w.Start, w.End)
		}
	}
}

func TestLocalAdapter_TimingsSpanDuration(t *testing.T) {
	a := &alignment.LocalAdapter{EstimatedBytesPerSecond: 1000}
	audio := make([]byte, 2000) // 2 seconds
	al, _ := a.Align(context.Background(), bytes.NewReader(audio), "a b c d")
	if len(al.Words) == 0 {
		t.Fatal("no words")
	}
	last := al.Words[len(al.Words)-1]
	// The last word's End should be close to the audio duration.
	if last.End < 1.9 || last.End > 2.1 {
		t.Errorf("last word End = %f, want ~2.0", last.End)
	}
}

func TestLocalAdapter_EmptyText_NoWords(t *testing.T) {
	a := &alignment.LocalAdapter{}
	al, err := a.Align(context.Background(), bytes.NewReader([]byte{1, 2, 3}), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(al.Words) != 0 {
		t.Errorf("expected 0 words for empty text, got %d", len(al.Words))
	}
}

func TestLocalAdapter_EmptyAudio_NonZeroDuration(t *testing.T) {
	a := &alignment.LocalAdapter{}
	al, err := a.Align(context.Background(), bytes.NewReader(nil), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(al.Words) != 1 {
		t.Fatalf("expected 1 word, got %d", len(al.Words))
	}
	if al.Words[0].End <= 0 {
		t.Errorf("End = %f should be positive even with empty audio", al.Words[0].End)
	}
}

func TestLocalAdapter_Deterministic(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	al1, _ := a.Align(context.Background(), bytes.NewReader(audio), "foo bar baz")
	al2, _ := a.Align(context.Background(), bytes.NewReader(audio), "foo bar baz")
	if len(al1.Words) != len(al2.Words) {
		t.Fatal("non-deterministic word count")
	}
	for i := range al1.Words {
		if al1.Words[i].Start != al2.Words[i].Start || al1.Words[i].End != al2.Words[i].End {
			t.Errorf("word[%d] timings differ between calls", i)
		}
	}
}

func TestLocalAdapter_StripsPunctuation(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	al, _ := a.Align(context.Background(), bytes.NewReader(audio), "hello, world.")
	if len(al.Words) != 2 {
		t.Fatalf("expected 2 words, got %d", len(al.Words))
	}
	if al.Words[0].Word != "hello" {
		t.Errorf("Words[0] = %q, want hello", al.Words[0].Word)
	}
	if al.Words[1].Word != "world" {
		t.Errorf("Words[1] = %q, want world", al.Words[1].Word)
	}
}

// ────────── AlignBytes ──────────

func TestAlignBytes_NoStore_ReturnsResult(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	res, err := alignment.AlignBytes(context.Background(), a, "chunk-001", audio, "hello world", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.ChunkID != "chunk-001" {
		t.Errorf("ChunkID = %q, want chunk-001", res.ChunkID)
	}
	if len(res.Alignment.Words) == 0 {
		t.Error("expected words in alignment result")
	}
}

// ────────── Restart-safe persistence ──────────

func TestAlignBytes_Persisted_ReloadedOnRestart(t *testing.T) {
	storeDir := t.TempDir()
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)

	// First call – should call aligner and persist.
	res1, err := alignment.AlignBytes(context.Background(), a, "chunk-001", audio, "hello world", storeDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file was written.
	entries, _ := os.ReadDir(storeDir)
	if len(entries) == 0 {
		t.Fatal("expected alignment JSON to be written to storeDir")
	}

	// Second call with a counter-provider to detect if aligner was skipped.
	counted := &countingAligner{inner: a}
	res2, err := alignment.AlignBytes(context.Background(), counted, "chunk-001", audio, "hello world", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if counted.calls > 0 {
		t.Errorf("aligner called %d times on second call; should use persisted result", counted.calls)
	}
	if len(res2.Alignment.Words) != len(res1.Alignment.Words) {
		t.Errorf("reloaded result has %d words, want %d", len(res2.Alignment.Words), len(res1.Alignment.Words))
	}
}

func TestLoadResult_NotFound_ReturnsError(t *testing.T) {
	_, err := alignment.LoadResult(t.TempDir(), "nonexistent")
	if err == nil {
		t.Error("expected error loading nonexistent alignment")
	}
}

func TestLoadResult_AfterPersist_Roundtrips(t *testing.T) {
	storeDir := t.TempDir()
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)

	alignment.AlignBytes(context.Background(), a, "chunk-xyz", audio, "one two", storeDir)

	res, err := alignment.LoadResult(storeDir, "chunk-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if res.ChunkID != "chunk-xyz" {
		t.Errorf("ChunkID = %q, want chunk-xyz", res.ChunkID)
	}
	if len(res.Alignment.Words) != 2 {
		t.Errorf("Words count = %d, want 2", len(res.Alignment.Words))
	}
}

func TestAlignChunk_UsesAudioFile(t *testing.T) {
	storeDir := t.TempDir()
	audioFile := filepath.Join(t.TempDir(), "test.mp3")
	// Write 16000 bytes as fake audio.
	if err := os.WriteFile(audioFile, make([]byte, 16000), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &alignment.LocalAdapter{}
	res, err := alignment.AlignChunk(context.Background(), a, "chunk-file", audioFile, "hello world", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Alignment.Words) == 0 {
		t.Error("expected words from file-based alignment")
	}
}

func TestAlignChunk_RestartSafe(t *testing.T) {
	storeDir := t.TempDir()
	audioFile := filepath.Join(t.TempDir(), "test.mp3")
	os.WriteFile(audioFile, make([]byte, 16000), 0o644)

	a := &alignment.LocalAdapter{}
	alignment.AlignChunk(context.Background(), a, "chunk-file", audioFile, "hello world", storeDir)

	counted := &countingAligner{inner: a}
	_, err := alignment.AlignChunk(context.Background(), counted, "chunk-file", audioFile, "hello world", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if counted.calls > 0 {
		t.Errorf("aligner called on restart; should use persisted result")
	}
}

// ────────── Helpers ──────────

type countingAligner struct {
	inner providers.Aligner
	calls int
}

func (c *countingAligner) Align(ctx context.Context, r io.Reader, text string) (providers.Alignment, error) {
	c.calls++
	return c.inner.Align(ctx, r, text)
}

// ────────── Schema version and fingerprint validation ──────────

func TestAlignBytes_ResultHasSchemaVersion(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	res, err := alignment.AlignBytes(context.Background(), a, "chunk-sv", audio, "hello world", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.SchemaVersion == 0 {
		t.Error("Result.SchemaVersion must not be zero")
	}
}

func TestAlignBytes_ResultHasFingerprints(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	res, err := alignment.AlignBytes(context.Background(), a, "chunk-fp", audio, "hello world", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.TranscriptFP == "" {
		t.Error("TranscriptFP must not be empty")
	}
	if res.AudioFP == "" {
		t.Error("AudioFP must not be empty")
	}
}

func TestAlignBytes_TranscriptFP_DiffersForDifferentText(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)
	res1, _ := alignment.AlignBytes(context.Background(), a, "c1", audio, "hello world", "")
	res2, _ := alignment.AlignBytes(context.Background(), a, "c2", audio, "goodbye world", "")
	if res1.TranscriptFP == res2.TranscriptFP {
		t.Error("different texts must produce different TranscriptFP")
	}
}

func TestAlignBytes_AudioFP_DiffersForDifferentAudio(t *testing.T) {
	a := &alignment.LocalAdapter{}
	audio1 := make([]byte, 16000)
	audio2 := make([]byte, 32000)
	res1, _ := alignment.AlignBytes(context.Background(), a, "c1", audio1, "hello", "")
	res2, _ := alignment.AlignBytes(context.Background(), a, "c2", audio2, "hello", "")
	if res1.AudioFP == res2.AudioFP {
		t.Error("different audio must produce different AudioFP")
	}
}

func TestLoadResult_StaleSchemaVersion_ReturnsError(t *testing.T) {
	storeDir := t.TempDir()
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)

	// Persist a result.
	alignment.AlignBytes(context.Background(), a, "chunk-schema", audio, "hello world", storeDir)

	// Corrupt the schema version in the JSON.
	p := storeDir + "/chunk-schema.json"
	raw, _ := os.ReadFile(p)
	corrupted := string(raw)
	corrupted = replaceInJSON(corrupted, `"schema_version": 1`, `"schema_version": 999`)
	os.WriteFile(p, []byte(corrupted), 0o644)

	_, err := alignment.LoadResult(storeDir, "chunk-schema")
	if err == nil {
		t.Error("expected error loading result with stale schema version")
	}
}

func TestAlignBytes_StaleSchemaVersion_ReAligns(t *testing.T) {
	storeDir := t.TempDir()
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)

	// Persist a result.
	alignment.AlignBytes(context.Background(), a, "chunk-stale", audio, "hello world", storeDir)

	// Corrupt schema version in cache file.
	p := storeDir + "/chunk-stale.json"
	raw, _ := os.ReadFile(p)
	corrupted := string(raw)
	corrupted = replaceInJSON(corrupted, `"schema_version": 1`, `"schema_version": 999`)
	os.WriteFile(p, []byte(corrupted), 0o644)

	// Second align must re-align (not use stale cache).
	counted := &countingAligner{inner: a}
	_, err := alignment.AlignBytes(context.Background(), counted, "chunk-stale", audio, "hello world", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if counted.calls == 0 {
		t.Error("aligner must be called again when cached schema version is stale")
	}
}

func TestAlignBytes_TranscriptChange_ReAligns(t *testing.T) {
	storeDir := t.TempDir()
	a := &alignment.LocalAdapter{}
	audio := make([]byte, 16000)

	// Persist alignment for "hello world".
	alignment.AlignBytes(context.Background(), a, "chunk-tc", audio, "hello world", storeDir)

	// Re-align with different text: cache must be invalid.
	counted := &countingAligner{inner: a}
	_, err := alignment.AlignBytes(context.Background(), counted, "chunk-tc", audio, "goodbye world", storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if counted.calls == 0 {
		t.Error("aligner must be called when transcript text changes")
	}
}

// ────────── Timing validation ──────────

func TestValidation_NegativeStart_ReturnsError(t *testing.T) {
	storeDir := t.TempDir()
	// Inject a result with a negative Start via a bad aligner.
	bad := &badTimingAligner{al: providers.Alignment{
		Words: []providers.WordTiming{{Word: "hi", Start: -1.0, End: 1.0}},
	}}
	_, err := alignment.AlignBytes(context.Background(), bad, "chunk-neg", make([]byte, 100), "hi", storeDir)
	if err == nil {
		t.Error("expected error for negative Start timing")
	}
}

func TestValidation_EndBeforeStart_ReturnsError(t *testing.T) {
	bad := &badTimingAligner{al: providers.Alignment{
		Words: []providers.WordTiming{{Word: "hi", Start: 2.0, End: 1.0}},
	}}
	_, err := alignment.AlignBytes(context.Background(), bad, "chunk-eb", make([]byte, 100), "hi", "")
	if err == nil {
		t.Error("expected error when End <= Start")
	}
}

func TestValidation_NonMonotonicStart_ReturnsError(t *testing.T) {
	bad := &badTimingAligner{al: providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "a", Start: 2.0, End: 3.0},
			{Word: "b", Start: 1.0, End: 2.5}, // Start goes backwards
		},
	}}
	_, err := alignment.AlignBytes(context.Background(), bad, "chunk-nm", make([]byte, 100), "a b", "")
	if err == nil {
		t.Error("expected error for non-monotonic Start timings")
	}
}

// ────────── Additional helpers ──────────

// badTimingAligner returns a fixed providers.Alignment regardless of input.
type badTimingAligner struct {
	al providers.Alignment
}

func (b *badTimingAligner) Align(_ context.Context, r io.Reader, _ string) (providers.Alignment, error) {
	io.Copy(io.Discard, r) // consume reader
	return b.al, nil
}

// replaceInJSON is a simple string replacement helper for test corruption.
func replaceInJSON(s, old, new string) string {
	idx := len(s)
	for i := 0; i < len(s)-len(old)+1; i++ {
		if s[i:i+len(old)] == old {
			idx = i
			break
		}
	}
	if idx == len(s) {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}
