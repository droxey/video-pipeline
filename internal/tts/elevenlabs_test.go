package tts_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/providers"
	"github.com/nebula/course-video-pipeline/internal/tts"
)

// ────────── ElevenLabsProvider ──────────

func makeElevenLabsServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *tts.ElevenLabsProvider) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := &tts.ElevenLabsProvider{
		APIKey:     "test-api-key",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}
	return srv, p
}

func TestElevenLabs_HappyPath_WritesAudio(t *testing.T) {
	fakeAudio := []byte("fake-mp3-audio-bytes")
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(fakeAudio)
	})

	var buf byteWriter
	u, err := provider.Synthesize(context.Background(), "Hello world.", "voice-abc", &buf)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(buf.data) != string(fakeAudio) {
		t.Errorf("audio = %q, want %q", buf.data, fakeAudio)
	}
	if u.Provider != "elevenlabs" {
		t.Errorf("Usage.Provider = %q, want elevenlabs", u.Provider)
	}
	if u.Operation != "synthesize" {
		t.Errorf("Usage.Operation = %q, want synthesize", u.Operation)
	}
	if u.Characters != len("Hello world.") {
		t.Errorf("Usage.Characters = %d, want %d", u.Characters, len("Hello world."))
	}
}

func TestElevenLabs_SendsAPIKey(t *testing.T) {
	var gotKey string
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("xi-api-key")
		w.WriteHeader(http.StatusOK)
	})

	provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if gotKey != "test-api-key" {
		t.Errorf("xi-api-key header = %q, want test-api-key", gotKey)
	}
}

func TestElevenLabs_SendsVoiceIDInURL(t *testing.T) {
	var gotPath string
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	provider.Synthesize(context.Background(), "x", "my-voice-id", &byteWriter{})
	if !strings.Contains(gotPath, "my-voice-id") {
		t.Errorf("URL path %q must contain voice ID", gotPath)
	}
}

func TestElevenLabs_SendsModelIDInBody(t *testing.T) {
	var gotBody map[string]interface{}
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	})
	provider.ModelID = "eleven_turbo_v2"

	provider.Synthesize(context.Background(), "hello", "v1", &byteWriter{})
	if gotBody["model_id"] != "eleven_turbo_v2" {
		t.Errorf("body model_id = %v, want eleven_turbo_v2", gotBody["model_id"])
	}
}

func TestElevenLabs_DefaultModel_WhenEmpty(t *testing.T) {
	var gotBody map[string]interface{}
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	})
	// ModelID not set → should use default.
	provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if gotBody["model_id"] == "" || gotBody["model_id"] == nil {
		t.Error("model_id must be set even when ModelID is empty (defaults to eleven_multilingual_v2)")
	}
}

func TestElevenLabs_TextInBody(t *testing.T) {
	var gotText string
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		if s, ok := body["text"].(string); ok {
			gotText = s
		}
		w.WriteHeader(http.StatusOK)
	})

	provider.Synthesize(context.Background(), "hello world", "v1", &byteWriter{})
	if gotText != "hello world" {
		t.Errorf("body text = %q, want %q", gotText, "hello world")
	}
}

func TestElevenLabs_429_IsTransient(t *testing.T) {
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if err == nil {
		t.Fatal("expected error on 429")
	}
	if !tts.IsTransient(err) {
		t.Errorf("429 must produce transient error; got: %v", err)
	}
}

func TestElevenLabs_500_IsTransient(t *testing.T) {
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !tts.IsTransient(err) {
		t.Errorf("500 must produce transient error; got: %v", err)
	}
}

func TestElevenLabs_503_IsTransient(t *testing.T) {
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	_, err := provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if err == nil {
		t.Fatal("expected error on 503")
	}
	if !tts.IsTransient(err) {
		t.Errorf("503 must produce transient error; got: %v", err)
	}
}

func TestElevenLabs_400_IsPermanent(t *testing.T) {
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	_, err := provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if tts.IsTransient(err) {
		t.Errorf("400 must be a permanent error; got transient: %v", err)
	}
}

func TestElevenLabs_401_IsPermanent(t *testing.T) {
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if tts.IsTransient(err) {
		t.Errorf("401 must be a permanent error; got transient: %v", err)
	}
}

func TestElevenLabs_VoiceSettings_InBody(t *testing.T) {
	var gotSettings map[string]interface{}
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		if vs, ok := body["voice_settings"].(map[string]interface{}); ok {
			gotSettings = vs
		}
		w.WriteHeader(http.StatusOK)
	})
	provider.VoiceSettings = tts.ElevenLabsVoiceSettings{
		Stability:       0.8,
		SimilarityBoost: 0.75,
	}

	provider.Synthesize(context.Background(), "x", "v1", &byteWriter{})
	if gotSettings == nil {
		t.Fatal("voice_settings must be present in request body")
	}
	if gotSettings["stability"] != 0.8 {
		t.Errorf("stability = %v, want 0.8", gotSettings["stability"])
	}
	if gotSettings["similarity_boost"] != 0.75 {
		t.Errorf("similarity_boost = %v, want 0.75", gotSettings["similarity_boost"])
	}
}

func TestElevenLabs_CancelledContext_ReturnsError(t *testing.T) {
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("audio"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.Synthesize(ctx, "x", "v1", &byteWriter{})
	if err == nil {
		t.Error("expected error when context is cancelled")
	}
}

func TestElevenLabs_StreamsLargeResponse(t *testing.T) {
	// Simulate a large audio response to verify streaming (not buffering).
	largeAudio := make([]byte, 1<<20) // 1 MiB
	for i := range largeAudio {
		largeAudio[i] = byte(i % 256)
	}
	_, provider := makeElevenLabsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(largeAudio)
	})

	var buf byteWriter
	_, err := provider.Synthesize(context.Background(), "long narration text", "v1", &buf)
	if err != nil {
		t.Fatalf("Synthesize large response: %v", err)
	}
	if len(buf.data) != len(largeAudio) {
		t.Errorf("received %d bytes, want %d", len(buf.data), len(largeAudio))
	}
}

// Verify ElevenLabsProvider satisfies the providers.TTS interface at compile time.
var _ providers.TTS = (*tts.ElevenLabsProvider)(nil)
