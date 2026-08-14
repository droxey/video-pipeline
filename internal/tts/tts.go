// Package tts wraps TTS providers with file-based caching, linear retry on
// transient errors, and usage accounting. No paid network calls are made in
// tests; use FakeProvider instead.
//
// Cache layout: for each chunk, two files are written atomically:
//
//	<cacheDir>/<chunkID>.mp3          – synthesized audio
//	<cacheDir>/<chunkID>.sidecar.json – versioned metadata (checksum, idempotency key, usage)
//
// A cache hit requires both files present, matching schema version, matching
// idempotency key (chunk ID + voice ID + text fingerprint), and matching
// audio SHA-256. Any mismatch triggers a full re-synthesis. Partial state
// (audio without sidecar, or sidecar without audio) is removed before each
// synthesis attempt.
package tts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/narration"
	"github.com/nebula/course-video-pipeline/internal/providers"
)

const ttsSidecarSchemaVersion = 1

// ErrTransient marks errors that are eligible for retry. Wrap this error in
// provider implementations to signal that the next attempt may succeed.
var ErrTransient = errors.New("transient error")

// IsTransient reports whether err (or any error in its chain) is transient.
func IsTransient(err error) bool { return errors.Is(err, ErrTransient) }

// TransientError constructs a transient error eligible for retry.
func TransientError(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrTransient)
}

// Sidecar is the versioned metadata file written alongside each cached audio
// file. It enables cache invalidation on schema upgrades, voice or text
// changes, and file corruption detected via SHA-256 mismatch.
type Sidecar struct {
	SchemaVersion   int    `json:"schema_version"`
	ChunkID         string `json:"chunk_id"`
	TextFingerprint string `json:"text_fingerprint"` // sha256(text) hex
	AudioSHA256     string `json:"audio_sha256"`     // sha256(audio file bytes) hex
	VoiceID         string `json:"voice_id"`
	Provider        string `json:"provider"`
	Operation       string `json:"operation"`
	Characters      int    `json:"characters"`
	// IdempotencyKey is sha256(chunkID + ":" + voiceID + ":" + textFingerprint).
	// Two calls with the same chunk/voice/text produce the same key, enabling
	// safe deduplication: if both writers produce the same audio the cache is
	// consistent; differing provider output implies a non-deterministic provider.
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// Options controls caching, retry, and voice selection for a synthesis call.
type Options struct {
	// CacheDir is the directory where synthesized audio files are stored, one
	// file per chunk ID. When empty no caching occurs.
	CacheDir string
	// MaxRetries is the number of additional attempts on transient errors.
	// Zero means try exactly once.
	MaxRetries int
	// VoiceID is forwarded verbatim to the provider's Synthesize call.
	VoiceID string
	// Backoff, when non-nil, is called before each retry with the 0-based
	// retry number (0 = first retry). The returned duration is awaited in a
	// context-aware select so cancellation aborts the sleep immediately.
	// nil means no sleep between retries.
	Backoff func(retry int) time.Duration
}

// AudioResult is the outcome of synthesizing a single narration chunk.
type AudioResult struct {
	ChunkID string
	// AudioPath is the path of the cached audio file. Empty when CacheDir is
	// not set.
	AudioPath string
	// Usage contains accounting metadata returned by the provider.
	Usage providers.Usage
	// FromCache is true when the audio was served from the file cache without
	// calling the provider.
	FromCache bool
	// Sidecar is the validated metadata loaded from or written alongside
	// AudioPath. Nil when CacheDir is empty.
	Sidecar *Sidecar
}

// Synthesize synthesizes audio for chunk via provider, applying file-based
// caching with validated sidecars and bounded retry on transient errors.
//
// Cache semantics: a hit requires a valid sidecar (schema version, idempotency
// key, and audio SHA-256 all match). Partial state (audio without sidecar, or
// vice versa) is cleaned up before each synthesis attempt so the next call
// starts fresh. The returned *domain.UsageRecord is nil on a cache hit.
func Synthesize(ctx context.Context, provider providers.TTS, chunk narration.Chunk, opts Options) (AudioResult, *domain.UsageRecord, error) {
	audioPath := ""
	if opts.CacheDir != "" {
		audioPath = filepath.Join(opts.CacheDir, chunk.ID+".mp3")
		scPath := SidecarPath(audioPath)
		textFP := textFingerprint(chunk.Text)
		idempKey := idempotencyKey(chunk.ID, opts.VoiceID, textFP)

		if sc, ok := loadValidSidecar(scPath, textFP, idempKey, audioPath); ok {
			return AudioResult{
				ChunkID:   chunk.ID,
				AudioPath: audioPath,
				FromCache: true,
				Sidecar:   sc,
			}, nil, nil
		}
		// Remove stale/partial state so doSynthesize starts clean.
		reconcileStaleCache(audioPath, scPath)
	}

	var u providers.Usage
	var lastErr error
	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return AudioResult{}, nil, ctx.Err()
		}
		if attempt > 0 && opts.Backoff != nil {
			d := opts.Backoff(attempt - 1)
			if d > 0 {
				select {
				case <-ctx.Done():
					return AudioResult{}, nil, ctx.Err()
				case <-time.After(d):
				}
			}
		}
		u, lastErr = doSynthesize(ctx, provider, chunk.Text, opts.VoiceID, audioPath)
		if lastErr == nil {
			break
		}
		if !IsTransient(lastErr) {
			break
		}
	}
	if lastErr != nil {
		return AudioResult{}, nil, fmt.Errorf("tts: chunk %s: %w", chunk.ID, lastErr)
	}

	var sc *Sidecar
	if audioPath != "" {
		audioSHA, err := fileSHA256(audioPath)
		if err != nil {
			return AudioResult{}, nil, fmt.Errorf("tts: checksum audio %s: %w", audioPath, err)
		}
		textFP := textFingerprint(chunk.Text)
		idempKey := idempotencyKey(chunk.ID, opts.VoiceID, textFP)
		sc = &Sidecar{
			SchemaVersion:   ttsSidecarSchemaVersion,
			ChunkID:         chunk.ID,
			TextFingerprint: textFP,
			AudioSHA256:     audioSHA,
			VoiceID:         opts.VoiceID,
			Provider:        u.Provider,
			Operation:       u.Operation,
			Characters:      u.Characters,
			IdempotencyKey:  idempKey,
			CreatedAt:       time.Now().UTC(),
		}
		if err := writeSidecar(SidecarPath(audioPath), sc); err != nil {
			return AudioResult{}, nil, err
		}
	}

	rec := &domain.UsageRecord{
		Provider:   u.Provider,
		Operation:  u.Operation,
		Characters: u.Characters,
		Voice:      opts.VoiceID,
		At:         time.Now().UTC(),
	}
	return AudioResult{ChunkID: chunk.ID, AudioPath: audioPath, Usage: u, Sidecar: sc}, rec, nil
}

// doSynthesize calls provider and writes audio to path (atomically via a temp
// file). When path is empty the audio is discarded.
func doSynthesize(ctx context.Context, provider providers.TTS, text, voiceID, path string) (providers.Usage, error) {
	if path == "" {
		return provider.Synthesize(ctx, text, voiceID, io.Discard)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return providers.Usage{}, fmt.Errorf("tts: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-tts-")
	if err != nil {
		return providers.Usage{}, fmt.Errorf("tts: create temp: %w", err)
	}
	tmpName := tmp.Name()
	u, synthErr := provider.Synthesize(ctx, text, voiceID, tmp)
	tmp.Close()
	if synthErr != nil {
		os.Remove(tmpName)
		return providers.Usage{}, synthErr
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return providers.Usage{}, fmt.Errorf("tts: rename to %s: %w", path, err)
	}
	return u, nil
}

// ────────── Sidecar helpers ──────────

// SidecarPath returns the sidecar path for the given audio path. Exported so
// tests can locate and manipulate sidecar files directly.
func SidecarPath(audioPath string) string {
	ext := filepath.Ext(audioPath)
	base := audioPath[:len(audioPath)-len(ext)]
	return base + ".sidecar.json"
}

func textFingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func idempotencyKey(chunkID, voiceID, textFP string) string {
	raw := chunkID + ":" + voiceID + ":" + textFP
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
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

// loadValidSidecar loads and validates the sidecar at scPath. Returns the
// sidecar and true only when all of the following hold:
//   - file exists and parses without error
//   - SchemaVersion == ttsSidecarSchemaVersion
//   - IdempotencyKey == idempKey (chunk ID + voice + text unchanged)
//   - audioPath exists and its SHA-256 matches sidecar.AudioSHA256 (not corrupted)
func loadValidSidecar(scPath, textFP, idempKey, audioPath string) (*Sidecar, bool) {
	data, err := os.ReadFile(scPath)
	if err != nil {
		return nil, false
	}
	var sc Sidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, false
	}
	if sc.SchemaVersion != ttsSidecarSchemaVersion {
		return nil, false
	}
	if sc.IdempotencyKey != idempKey {
		return nil, false
	}
	actualSHA, err := fileSHA256(audioPath)
	if err != nil {
		return nil, false
	}
	if actualSHA != sc.AudioSHA256 {
		return nil, false
	}
	return &sc, true
}

// reconcileStaleCache removes stale or partial cache files. Called when sidecar
// validation fails so a subsequent synthesis starts from a clean state.
func reconcileStaleCache(audioPath, scPath string) {
	os.Remove(audioPath)
	os.Remove(scPath)
}

// writeSidecar atomically writes sc as pretty-printed JSON to path.
func writeSidecar(path string, sc *Sidecar) error {
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return fmt.Errorf("tts: marshal sidecar: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-sidecar-")
	if err != nil {
		return fmt.Errorf("tts: create sidecar temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("tts: write sidecar: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("tts: sync sidecar: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("tts: close sidecar: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("tts: rename sidecar to %s: %w", path, err)
	}
	return nil
}
