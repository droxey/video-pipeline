// Package alignment wraps forced-alignment providers with restart-safe
// persistence. Alignment results are cached as JSON files keyed by chunk ID so
// that a resumed run never re-aligns audio that has already been processed.
// No paid network calls or real audio processing occur in tests; use
// LocalAdapter instead.
//
// # Validation
//
// Every Result is validated before being persisted and when loaded from cache:
//   - SchemaVersion must match alignmentSchemaVersion.
//   - TranscriptFP must equal sha256(text) confirming the cached text matches.
//   - All word timings must satisfy Start >= 0, End > Start, and starts must be
//     monotonically non-decreasing (consecutive words do not go backwards).
//   - All timing values must be finite (not NaN or ±Inf).
//
// A cache entry that fails validation is silently discarded and the aligner is
// called again; the corrupt entry is overwritten atomically.
package alignment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebula/course-video-pipeline/internal/persistence"
	"github.com/nebula/course-video-pipeline/internal/providers"
)

const alignmentSchemaVersion = 1

// Result is the outcome of aligning one audio file to its transcript.
// It extends providers.Alignment with source identifiers and versioned
// metadata needed for cache validation.
type Result struct {
	SchemaVersion int    `json:"schema_version"`
	ChunkID       string `json:"chunk_id"`
	AudioPath     string `json:"audio_path"`
	// TranscriptFP is sha256(text) hex. On cache load it is compared against
	// the current text to detect transcript changes.
	TranscriptFP string `json:"transcript_fingerprint"`
	// AudioFP is sha256(audio bytes) hex. It captures the exact audio that
	// produced this alignment, enabling downstream checksum chains.
	AudioFP   string              `json:"audio_fingerprint"`
	Alignment providers.Alignment `json:"alignment"`
}

// AlignChunk aligns the audio at audioPath to transcript text using aligner.
// When storeDir is non-empty the result is persisted atomically to
// storeDir/<chunkID>.json and reloaded on the next call without invoking the
// aligner again (restart-safe). Cached results are validated; corrupt or
// schema-incompatible entries are discarded and re-computed.
func AlignChunk(ctx context.Context, aligner providers.Aligner, chunkID, audioPath, text, storeDir string) (Result, error) {
	if storeDir != "" {
		if cached, ok := loadValidCached(storeDir, chunkID, text); ok {
			return cached, nil
		}
	}

	f, err := os.Open(audioPath)
	if err != nil {
		return Result{}, fmt.Errorf("alignment: open %s: %w", audioPath, err)
	}
	defer f.Close()

	h := sha256.New()
	tr := io.TeeReader(f, h)
	al, err := aligner.Align(ctx, tr, text)
	if err != nil {
		return Result{}, fmt.Errorf("alignment: align chunk %s: %w", chunkID, err)
	}
	if err := validateAlignmentTimings(al); err != nil {
		return Result{}, fmt.Errorf("alignment: invalid timings for chunk %s: %w", chunkID, err)
	}

	res := Result{
		SchemaVersion: alignmentSchemaVersion,
		ChunkID:       chunkID,
		AudioPath:     audioPath,
		TranscriptFP:  transcriptFingerprint(text),
		AudioFP:       hex.EncodeToString(h.Sum(nil)),
		Alignment:     al,
	}
	if storeDir != "" {
		if err := persist(storeDir, res); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

// AlignReader aligns audio from r to transcript text using aligner.
// storeDir/chunkID caching applies when storeDir is non-empty.
func AlignReader(ctx context.Context, aligner providers.Aligner, chunkID string, r io.Reader, text, storeDir string) (Result, error) {
	if storeDir != "" {
		if cached, ok := loadValidCached(storeDir, chunkID, text); ok {
			return cached, nil
		}
	}

	h := sha256.New()
	tr := io.TeeReader(r, h)
	al, err := aligner.Align(ctx, tr, text)
	if err != nil {
		return Result{}, fmt.Errorf("alignment: align reader chunk %s: %w", chunkID, err)
	}
	if err := validateAlignmentTimings(al); err != nil {
		return Result{}, fmt.Errorf("alignment: invalid timings for chunk %s: %w", chunkID, err)
	}

	res := Result{
		SchemaVersion: alignmentSchemaVersion,
		ChunkID:       chunkID,
		AudioPath:     "",
		TranscriptFP:  transcriptFingerprint(text),
		AudioFP:       hex.EncodeToString(h.Sum(nil)),
		Alignment:     al,
	}
	if storeDir != "" {
		if err := persist(storeDir, res); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

// LoadResult reads a previously persisted alignment result from storeDir.
// It validates the schema version and timing constraints but does not
// re-verify the transcript fingerprint (the original text is not available).
func LoadResult(storeDir, chunkID string) (Result, error) {
	var res Result
	if err := persistence.ReadJSON(resultPath(storeDir, chunkID), &res); err != nil {
		return Result{}, fmt.Errorf("alignment: load %s: %w", chunkID, err)
	}
	if res.SchemaVersion != alignmentSchemaVersion {
		return Result{}, fmt.Errorf("alignment: schema version mismatch for %s: got %d, want %d",
			chunkID, res.SchemaVersion, alignmentSchemaVersion)
	}
	if err := validateAlignmentTimings(res.Alignment); err != nil {
		return Result{}, fmt.Errorf("alignment: corrupt timings for %s: %w", chunkID, err)
	}
	return res, nil
}

// loadValidCached loads and fully validates a cached result for chunkID/text.
// Returns (result, true) only when schema version, transcript fingerprint, and
// timing constraints all pass; otherwise returns (zero, false).
func loadValidCached(storeDir, chunkID, text string) (Result, bool) {
	var res Result
	if err := persistence.ReadJSON(resultPath(storeDir, chunkID), &res); err != nil {
		return Result{}, false
	}
	if res.SchemaVersion != alignmentSchemaVersion {
		return Result{}, false
	}
	if res.TranscriptFP != transcriptFingerprint(text) {
		return Result{}, false
	}
	if err := validateAlignmentTimings(res.Alignment); err != nil {
		return Result{}, false
	}
	return res, true
}

// validateAlignmentTimings checks that all word timings in al are finite,
// non-negative, strictly increasing within each word (End > Start), and
// monotonically non-decreasing across words (Start[i] >= Start[i-1]).
func validateAlignmentTimings(al providers.Alignment) error {
	if math.IsNaN(al.Loss) || math.IsInf(al.Loss, 0) {
		return fmt.Errorf("alignment loss is not finite: %f", al.Loss)
	}
	for i, w := range al.Words {
		if math.IsNaN(w.Start) || math.IsInf(w.Start, 0) {
			return fmt.Errorf("word[%d] %q: non-finite Start=%f", i, w.Word, w.Start)
		}
		if math.IsNaN(w.End) || math.IsInf(w.End, 0) {
			return fmt.Errorf("word[%d] %q: non-finite End=%f", i, w.Word, w.End)
		}
		if w.Start < 0 {
			return fmt.Errorf("word[%d] %q: negative Start=%f", i, w.Word, w.Start)
		}
		if w.End <= w.Start {
			return fmt.Errorf("word[%d] %q: End(%f) <= Start(%f)", i, w.Word, w.End, w.Start)
		}
		if i > 0 && w.Start < al.Words[i-1].Start {
			return fmt.Errorf("word[%d] %q: Start(%f) < word[%d] Start(%f) (non-monotonic)",
				i, w.Word, w.Start, i-1, al.Words[i-1].Start)
		}
	}
	return nil
}

func persist(storeDir string, res Result) error {
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return fmt.Errorf("alignment: mkdir %s: %w", storeDir, err)
	}
	if err := persistence.WriteJSON(resultPath(storeDir, res.ChunkID), res); err != nil {
		return fmt.Errorf("alignment: persist %s: %w", res.ChunkID, err)
	}
	return nil
}

func resultPath(storeDir, chunkID string) string {
	return filepath.Join(storeDir, chunkID+".json")
}

func transcriptFingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// ────────── LocalAdapter ──────────

// LocalAdapter is a deterministic, executable-free Aligner for tests and
// offline use. It reads all bytes from the audio reader to derive a duration
// estimate, then distributes word timings evenly across that duration.
//
// Duration estimate: bytesRead / EstimatedBytesPerSecond.
// The default EstimatedBytesPerSecond is 16000 (16-bit mono 8 kHz).
//
// Timings produced by LocalAdapter satisfy all constraints in
// validateAlignmentTimings: finite, non-negative, End > Start, monotonic.
type LocalAdapter struct {
	// EstimatedBytesPerSecond controls how audio byte count is converted to
	// duration. Must be positive. Defaults to 16000 when zero.
	EstimatedBytesPerSecond int
}

// Align implements providers.Aligner deterministically without any subprocess.
func (a *LocalAdapter) Align(_ context.Context, r io.Reader, text string) (providers.Alignment, error) {
	bps := a.EstimatedBytesPerSecond
	if bps <= 0 {
		bps = 16000
	}

	// Consume the reader to learn the byte count.
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		return providers.Alignment{}, fmt.Errorf("local aligner: read audio: %w", err)
	}

	duration := float64(n) / float64(bps)
	if duration <= 0 {
		duration = 1.0 // avoid zero-duration for empty audio
	}

	words := tokenizeWords(text)
	if len(words) == 0 {
		return providers.Alignment{Loss: 0}, nil
	}

	interval := duration / float64(len(words))
	timings := make([]providers.WordTiming, len(words))
	for i, w := range words {
		timings[i] = providers.WordTiming{
			Word:  w,
			Start: float64(i) * interval,
			End:   float64(i+1) * interval,
		}
	}
	return providers.Alignment{Words: timings, Loss: 0}, nil
}

// tokenizeWords splits text on whitespace and returns non-empty tokens.
func tokenizeWords(text string) []string {
	raw := strings.Fields(text)
	out := raw[:0]
	for _, w := range raw {
		w = strings.Trim(w, ".,!?;:")
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// AlignBytes is a convenience wrapper that aligns audio supplied as a byte
// slice. Useful in tests where audio comes from a buffer rather than a file.
func AlignBytes(ctx context.Context, aligner providers.Aligner, chunkID string, audio []byte, text, storeDir string) (Result, error) {
	return AlignReader(ctx, aligner, chunkID, bytes.NewReader(audio), text, storeDir)
}
