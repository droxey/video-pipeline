package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nebula/course-video-pipeline/internal/providers"
)

const (
	elevenlabsDefaultBaseURL = "https://api.elevenlabs.io"
	elevenlabsDefaultModel   = "eleven_multilingual_v2"
	// elevenlabsStreamPath is the streaming TTS endpoint. Voice ID is substituted at call time.
	elevenlabsStreamPath = "/v1/text-to-speech/%s/stream"
)

// ElevenLabsProvider implements providers.TTS using the ElevenLabs HTTP API.
//
// It uses the streaming endpoint so that large synthesized audio is written
// incrementally to the output writer rather than buffered in memory.
//
// HTTP error classification:
//   - 429 Too Many Requests → ErrTransient (rate-limited, eligible for retry)
//   - 5xx Server Error      → ErrTransient (server-side failure, retry-eligible)
//   - Other 4xx             → permanent error (bad request, auth failure, etc.)
//
// For offline/test use, substitute FakeProvider instead. No real API key is
// required in that case.
type ElevenLabsProvider struct {
	// APIKey is the ElevenLabs API key forwarded in the xi-api-key header.
	APIKey string
	// ModelID selects the synthesis model. Defaults to elevenlabsDefaultModel.
	ModelID string
	// BaseURL overrides the API base URL. Defaults to elevenlabsDefaultBaseURL.
	// Override in tests by pointing to an httptest.Server.
	BaseURL string
	// HTTPClient is used for all requests. Defaults to a client with a 30-second
	// timeout when nil.
	HTTPClient *http.Client
	// VoiceSettings overrides per-request voice settings. Zero value uses ElevenLabs defaults.
	VoiceSettings ElevenLabsVoiceSettings
}

// ElevenLabsVoiceSettings holds the optional per-request voice tuning parameters.
type ElevenLabsVoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style,omitempty"`
	UseSpeakerBoost bool    `json:"use_speaker_boost,omitempty"`
}

// elevenlabsRequest is the canonical JSON body sent to the ElevenLabs API.
type elevenlabsRequest struct {
	Text          string                  `json:"text"`
	ModelID       string                  `json:"model_id"`
	VoiceSettings ElevenLabsVoiceSettings `json:"voice_settings"`
}

// elevenlabsCharacterCost approximates the cost of a request for usage accounting.
// ElevenLabs charges per character; this value mirrors that unit.
const elevenlabsCharacterCost = 1

// Synthesize implements providers.TTS. It posts a streaming synthesis request
// to ElevenLabs and writes the MP3 audio response to w.
//
// The returned Usage.Characters is len(text). The request ID from the
// x-request-id response header is not currently exposed via the Usage struct
// but is available in the HTTP response (reserved for future structured logging).
func (e *ElevenLabsProvider) Synthesize(ctx context.Context, text, voiceID string, w io.Writer) (providers.Usage, error) {
	model := e.ModelID
	if model == "" {
		model = elevenlabsDefaultModel
	}
	base := e.BaseURL
	if base == "" {
		base = elevenlabsDefaultBaseURL
	}
	client := e.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	url := base + fmt.Sprintf(elevenlabsStreamPath, voiceID)

	reqBody := elevenlabsRequest{
		Text:          text,
		ModelID:       model,
		VoiceSettings: e.VoiceSettings,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return providers.Usage{}, fmt.Errorf("elevenlabs: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return providers.Usage{}, fmt.Errorf("elevenlabs: build request: %w", err)
	}
	req.Header.Set("xi-api-key", e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := client.Do(req)
	if err != nil {
		// Network errors (connection refused, timeout) are transient.
		return providers.Usage{}, fmt.Errorf("elevenlabs: do request: %w: %w", err, ErrTransient)
	}
	defer resp.Body.Close()

	if err := elevenlabsClassifyStatus(resp.StatusCode); err != nil {
		// Drain body to allow connection reuse.
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return providers.Usage{}, fmt.Errorf("elevenlabs: HTTP %d: %w", resp.StatusCode, err)
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return providers.Usage{}, fmt.Errorf("elevenlabs: stream response: %w", err)
	}

	return providers.Usage{
		Provider:   "elevenlabs",
		Operation:  "synthesize",
		Characters: len(text),
	}, nil
}

// elevenlabsClassifyStatus converts an HTTP status code to an error. Returns
// nil for 2xx. Returns a transient error for 429 and 5xx. Returns a permanent
// error for all other non-2xx codes.
func elevenlabsClassifyStatus(code int) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusTooManyRequests:
		return fmt.Errorf("rate limited: %w", ErrTransient)
	case code >= 500:
		return fmt.Errorf("server error: %w", ErrTransient)
	default:
		return fmt.Errorf("client error (status %d)", code)
	}
}
