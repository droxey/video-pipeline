package tts

import (
	"context"
	"fmt"
	"io"

	"github.com/nebula/course-video-pipeline/internal/providers"
)

// FakeProvider is a deterministic, in-process TTS provider for tests.
// It never makes network calls. For each text it writes a short deterministic
// byte sequence so tests can verify caching and accounting without real audio.
type FakeProvider struct {
	// BytesPerChar controls synthetic audio size. Defaults to 2 when zero.
	BytesPerChar int
	// ForceErr, when non-nil, is returned from every Synthesize call.
	// Wrap ErrTransient to test retry logic: fmt.Errorf("…: %w", ErrTransient)
	ForceErr error
	// CallCount is incremented on each Synthesize call (including retried ones).
	CallCount int
}

// Synthesize implements providers.TTS using deterministic in-memory generation.
func (f *FakeProvider) Synthesize(_ context.Context, text, voiceID string, w io.Writer) (providers.Usage, error) {
	f.CallCount++
	if f.ForceErr != nil {
		return providers.Usage{}, f.ForceErr
	}
	bpc := f.BytesPerChar
	if bpc <= 0 {
		bpc = 2
	}
	buf := deterministicBytes(text, bpc)
	if _, err := w.Write(buf); err != nil {
		return providers.Usage{}, fmt.Errorf("fake tts: write: %w", err)
	}
	return providers.Usage{
		Provider:   "fake",
		Operation:  "synthesize",
		Characters: len(text),
	}, nil
}

// deterministicBytes generates len(text)*bpc bytes that are fully determined
// by the text content using a linear-congruential generator seeded from text.
func deterministicBytes(text string, bpc int) []byte {
	// Seed from text so same input always yields same bytes.
	var seed uint64
	for i, b := range []byte(text) {
		seed ^= uint64(b) << (uint(i%8) * 8)
	}
	buf := make([]byte, len(text)*bpc)
	for i := range buf {
		// LCG parameters from Knuth.
		seed = seed*6364136223846793005 + 1442695040888963407
		buf[i] = byte(seed >> 56)
	}
	return buf
}
