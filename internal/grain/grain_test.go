package grain_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/grain"
	"github.com/nebula/course-video-pipeline/internal/quality"
)

func TestParseTranscriptJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "grain", "transcript.json"))
	if err != nil {
		t.Fatal(err)
	}

	utterances, err := grain.ParseTranscriptJSON(data)
	if err != nil {
		t.Fatalf("ParseTranscriptJSON: %v", err)
	}
	if len(utterances) != 4 {
		t.Fatalf("expected 4 utterances, got %d", len(utterances))
	}
	if utterances[0].Start != 330 {
		t.Errorf("first start = %d, want 330 ms", utterances[0].Start)
	}
}

func TestToRecording_msToSecondsAndSegments(t *testing.T) {
	utterances := []grain.Utterance{
		{Start: 330, End: 4200, Text: "Hello.", ParticipantID: "p-1", Speaker: "Alice"},
		{Start: 4500, End: 9100, Text: "Hi there.", ParticipantID: "p-2", Speaker: "Bob"},
	}

	rec, err := grain.ToRecording(grain.Meta{
		RecordingID: "rec-test",
		Title:       "Test",
		RecordedAt:  time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
	}, utterances, grain.Options{})
	if err != nil {
		t.Fatal(err)
	}

	if rec.Segments[0].Start != 0.33 {
		t.Errorf("segment 0 start = %v, want 0.33", rec.Segments[0].Start)
	}
	if rec.Segments[0].End != 4.2 {
		t.Errorf("segment 0 end = %v, want 4.2", rec.Segments[0].End)
	}
	if rec.Segments[1].Start != 4.5 {
		t.Errorf("segment 1 start = %v, want 4.5", rec.Segments[1].Start)
	}
	if rec.DurationSeconds != 9.1 {
		t.Errorf("duration = %v, want 9.1", rec.DurationSeconds)
	}
}

func TestToRecording_speakerDedupe(t *testing.T) {
	utterances := []grain.Utterance{
		{Start: 0, End: 1000, Text: "One.", ParticipantID: "p-abc", Speaker: "Dani Roxberry"},
		{Start: 1100, End: 2000, Text: "Two.", ParticipantID: "p-def", Speaker: "Bob Chen"},
		{Start: 2100, End: 3000, Text: "Three.", ParticipantID: "p-abc", Speaker: "Dani Roxberry"},
	}

	rec, err := grain.ToRecording(grain.Meta{RecordingID: "rec-dedupe"}, utterances, grain.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Speakers) != 2 {
		t.Fatalf("expected 2 speakers, got %d", len(rec.Speakers))
	}
	if rec.Speakers[0].SourceID != "p-abc" || rec.Speakers[1].SourceID != "p-def" {
		t.Errorf("unexpected speaker order: %+v", rec.Speakers)
	}
}

func TestToRecording_narratorByFlag(t *testing.T) {
	utterances := []grain.Utterance{
		{Start: 0, End: 1000, Text: "Bob speaks first.", ParticipantID: "p-def", Speaker: "Bob Chen"},
		{Start: 1100, End: 2000, Text: "Dani responds.", ParticipantID: "p-abc", Speaker: "Dani Roxberry"},
	}

	rec, err := grain.ToRecording(
		grain.Meta{RecordingID: "rec-narrator-flag"},
		utterances,
		grain.Options{Narrator: "Dani Roxberry"},
	)
	if err != nil {
		t.Fatal(err)
	}

	roleByID := map[string]domain.SpeakerRole{}
	for _, sp := range rec.Speakers {
		roleByID[sp.SourceID] = sp.Role
	}
	if roleByID["p-abc"] != domain.SpeakerRoleNarrator {
		t.Errorf("Dani should be narrator, got %q", roleByID["p-abc"])
	}
	if roleByID["p-def"] != domain.SpeakerRoleParticipant {
		t.Errorf("Bob should be participant, got %q", roleByID["p-def"])
	}
}

func TestToRecording_narratorSingleSpeaker(t *testing.T) {
	utterances := []grain.Utterance{
		{Start: 0, End: 1000, Text: "Solo.", ParticipantID: "p-solo", Speaker: "Solo Speaker"},
	}

	rec, err := grain.ToRecording(grain.Meta{RecordingID: "rec-solo"}, utterances, grain.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Speakers) != 1 {
		t.Fatalf("expected 1 speaker, got %d", len(rec.Speakers))
	}
	if rec.Speakers[0].Role != domain.SpeakerRoleNarrator {
		t.Errorf("sole speaker role = %q, want narrator", rec.Speakers[0].Role)
	}
}

func TestToRecording_narratorFirstSpeakerFallback(t *testing.T) {
	utterances := []grain.Utterance{
		{Start: 0, End: 1000, Text: "First.", ParticipantID: "p-first", Speaker: "Alice"},
		{Start: 1100, End: 2000, Text: "Second.", ParticipantID: "p-second", Speaker: "Bob"},
	}

	rec, err := grain.ToRecording(grain.Meta{RecordingID: "rec-fallback"}, utterances, grain.Options{})
	if err != nil {
		t.Fatal(err)
	}

	if rec.Speakers[0].Role != domain.SpeakerRoleNarrator {
		t.Errorf("first speaker role = %q, want narrator", rec.Speakers[0].Role)
	}
	if rec.Speakers[1].Role != domain.SpeakerRoleParticipant {
		t.Errorf("second speaker role = %q, want participant", rec.Speakers[1].Role)
	}
}

func TestToRecording_segmentOrdering(t *testing.T) {
	// Out-of-order input should be sorted by start time.
	utterances := []grain.Utterance{
		{Start: 5000, End: 6000, Text: "Later.", ParticipantID: "p-1", Speaker: "A"},
		{Start: 1000, End: 2000, Text: "Earlier.", ParticipantID: "p-1", Speaker: "A"},
	}

	rec, err := grain.ToRecording(grain.Meta{RecordingID: "rec-order"}, utterances, grain.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Segments[0].Text != "Earlier." || rec.Segments[1].Text != "Later." {
		t.Errorf("segments not sorted: %+v", rec.Segments)
	}
	if rec.Segments[1].Start <= rec.Segments[0].Start {
		t.Errorf("segment order invalid: starts %v then %v", rec.Segments[0].Start, rec.Segments[1].Start)
	}
}

func TestToRecording_fixturePassesValidation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "grain", "transcript.json"))
	if err != nil {
		t.Fatal(err)
	}
	utterances, err := grain.ParseTranscriptJSON(data)
	if err != nil {
		t.Fatal(err)
	}

	rec, err := grain.ToRecording(grain.Meta{
		RecordingID: "grain-rec-00abc123",
		Title:       "Event-Driven Architecture Deep Dive",
		SourceURL:   "https://grain.com/share/grain-rec-00abc123",
		RecordedAt:  time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
	}, utterances, grain.Options{Narrator: "Dani Roxberry"})
	if err != nil {
		t.Fatal(err)
	}

	if rec.Source != "grain" {
		t.Errorf("source = %q, want grain", rec.Source)
	}
	if errs := quality.ValidateRecording(rec, false); len(errs) != 0 {
		t.Errorf("validation errors: %v", errs)
	}
}

func TestToRecording_durationOverride(t *testing.T) {
	override := 99.9
	utterances := []grain.Utterance{
		{Start: 0, End: 1000, Text: "Hi.", ParticipantID: "p-1", Speaker: "A"},
	}

	rec, err := grain.ToRecording(
		grain.Meta{RecordingID: "rec-dur", DurationSeconds: &override},
		utterances,
		grain.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if rec.DurationSeconds != 99.9 {
		t.Errorf("duration = %v, want override 99.9", rec.DurationSeconds)
	}
}
