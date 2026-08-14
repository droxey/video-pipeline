package slides_test

import (
	"os"
	"strings"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/slides"
)

const slidesMD = `## Slide 1: Introduction

<!-- duration: 30.0 -->
<!-- sfx: chime-start -->

Event-Driven Architecture is a design paradigm.

## Slide 2: Benefits

<!-- duration: 45.0 -->
<!-- sfx: emphasis -->

Key benefits include loose coupling and scalability.
`

const notesMD = `## Slide 1

<!-- source: seg-0001 -->
<!-- approved-by: alice -->
<!-- approved-at: 2026-08-01T10:00:00Z -->

Welcome to the course. [PAUSE:0.5] Let us begin.

## Slide 2

<!-- source: seg-0002, seg-0003 -->

Great question. [SFX:emphasis] Direct coupling is brittle.
`

// ────────── ParseSlides ──────────

func TestParseSlides_Count(t *testing.T) {
	parsed, errs := slides.ParseSlides(slidesMD)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := len(parsed); got != 2 {
		t.Fatalf("slide count = %d, want 2", got)
	}
}

func TestParseSlides_NumberAndTitle(t *testing.T) {
	parsed, _ := slides.ParseSlides(slidesMD)
	if parsed[0].Number != 1 {
		t.Errorf("slide[0].Number = %d, want 1", parsed[0].Number)
	}
	if parsed[0].Title != "Introduction" {
		t.Errorf("slide[0].Title = %q, want %q", parsed[0].Title, "Introduction")
	}
	if parsed[1].Number != 2 {
		t.Errorf("slide[1].Number = %d, want 2", parsed[1].Number)
	}
}

func TestParseSlides_Duration(t *testing.T) {
	parsed, _ := slides.ParseSlides(slidesMD)
	if parsed[0].DurationSeconds != 30.0 {
		t.Errorf("slide[0].DurationSeconds = %f, want 30.0", parsed[0].DurationSeconds)
	}
	if parsed[1].DurationSeconds != 45.0 {
		t.Errorf("slide[1].DurationSeconds = %f, want 45.0", parsed[1].DurationSeconds)
	}
}

func TestParseSlides_SFX(t *testing.T) {
	parsed, _ := slides.ParseSlides(slidesMD)
	if len(parsed[0].SFX) != 1 {
		t.Fatalf("slide[0].SFX len = %d, want 1", len(parsed[0].SFX))
	}
	if parsed[0].SFX[0].Preset != "chime-start" {
		t.Errorf("slide[0].SFX[0].Preset = %q, want chime-start", parsed[0].SFX[0].Preset)
	}
}

func TestParseSlides_BodyStripsMetadata(t *testing.T) {
	parsed, _ := slides.ParseSlides(slidesMD)
	if strings.Contains(parsed[0].Body, "duration:") {
		t.Error("body should not contain duration metadata comment")
	}
	if strings.Contains(parsed[0].Body, "sfx:") {
		t.Error("body should not contain sfx metadata comment")
	}
	if parsed[0].Body == "" {
		t.Error("body must not be empty")
	}
}

func TestParseSlides_NormalizedHash(t *testing.T) {
	parsed, _ := slides.ParseSlides(slidesMD)
	if parsed[0].NormalizedHash == "" {
		t.Error("NormalizedHash must not be empty")
	}
	if len(parsed[0].NormalizedHash) != 64 {
		t.Errorf("NormalizedHash len = %d, want 64 (sha256 hex)", len(parsed[0].NormalizedHash))
	}
}

func TestParseSlides_HashDeterministic(t *testing.T) {
	p1, _ := slides.ParseSlides(slidesMD)
	p2, _ := slides.ParseSlides(slidesMD)
	if p1[0].NormalizedHash != p2[0].NormalizedHash {
		t.Error("NormalizedHash must be deterministic")
	}
}

func TestParseSlides_HashChangesWithBody(t *testing.T) {
	p1, _ := slides.ParseSlides(slidesMD)
	modified := strings.Replace(slidesMD, "design paradigm", "different text", 1)
	p2, _ := slides.ParseSlides(modified)
	if p1[0].NormalizedHash == p2[0].NormalizedHash {
		t.Error("different body text must produce different hash")
	}
}

func TestParseSlides_WithFixture(t *testing.T) {
	data, err := os.ReadFile("../../testdata/fixtures/slides.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, errs := slides.ParseSlides(string(data))
	if len(errs) != 0 {
		t.Errorf("fixture parse errors: %v", errs)
	}
	if len(parsed) != 3 {
		t.Errorf("fixture slide count = %d, want 3", len(parsed))
	}
}

// ────────── ParseNotes ──────────

func TestParseNotes_Count(t *testing.T) {
	parsed, errs := slides.ParseNotes(notesMD)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := len(parsed); got != 2 {
		t.Fatalf("note count = %d, want 2", got)
	}
}

func TestParseNotes_SlideNumber(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	if parsed[0].SlideNumber != 1 {
		t.Errorf("note[0].SlideNumber = %d, want 1", parsed[0].SlideNumber)
	}
	if parsed[1].SlideNumber != 2 {
		t.Errorf("note[1].SlideNumber = %d, want 2", parsed[1].SlideNumber)
	}
}

func TestParseNotes_SourceIDs(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	if len(parsed[0].SourceIDs) != 1 || parsed[0].SourceIDs[0] != "seg-0001" {
		t.Errorf("note[0].SourceIDs = %v, want [seg-0001]", parsed[0].SourceIDs)
	}
	if len(parsed[1].SourceIDs) != 2 {
		t.Fatalf("note[1].SourceIDs len = %d, want 2", len(parsed[1].SourceIDs))
	}
	if parsed[1].SourceIDs[0] != "seg-0002" || parsed[1].SourceIDs[1] != "seg-0003" {
		t.Errorf("note[1].SourceIDs = %v, want [seg-0002 seg-0003]", parsed[1].SourceIDs)
	}
}

func TestParseNotes_Approval(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	if parsed[0].ApprovedBy != "alice" {
		t.Errorf("note[0].ApprovedBy = %q, want alice", parsed[0].ApprovedBy)
	}
	if parsed[0].ApprovedAt == nil {
		t.Fatal("note[0].ApprovedAt must not be nil")
	}
	if parsed[0].ApprovedAt.Year() != 2026 {
		t.Errorf("ApprovedAt year = %d, want 2026", parsed[0].ApprovedAt.Year())
	}
}

func TestParseNotes_CleanTextStripsMarkers(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	// PAUSE marker must be stripped.
	if strings.Contains(parsed[0].CleanText, "[PAUSE") {
		t.Errorf("CleanText still contains PAUSE marker: %q", parsed[0].CleanText)
	}
	// SFX marker must be stripped.
	if strings.Contains(parsed[1].CleanText, "[SFX") {
		t.Errorf("CleanText still contains SFX marker: %q", parsed[1].CleanText)
	}
	if parsed[0].CleanText == "" {
		t.Error("CleanText must not be empty after stripping")
	}
}

func TestParseNotes_PauseMarkersExtracted(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	if len(parsed[0].Pauses) != 1 {
		t.Fatalf("note[0].Pauses len = %d, want 1", len(parsed[0].Pauses))
	}
	if parsed[0].Pauses[0].Seconds != 0.5 {
		t.Errorf("pause seconds = %f, want 0.5", parsed[0].Pauses[0].Seconds)
	}
}

func TestParseNotes_SFXMarkersExtracted(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	if len(parsed[1].SFX) != 1 {
		t.Fatalf("note[1].SFX len = %d, want 1", len(parsed[1].SFX))
	}
	if parsed[1].SFX[0].Preset != "emphasis" {
		t.Errorf("SFX preset = %q, want emphasis", parsed[1].SFX[0].Preset)
	}
}

func TestParseNotes_SFXPositionInCleanText(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	// SFX position must be a valid byte offset in the clean text.
	pos := parsed[1].SFX[0].Position
	if pos < 0 || pos > len(parsed[1].CleanText) {
		t.Errorf("SFX position %d out of range [0, %d]", pos, len(parsed[1].CleanText))
	}
}

func TestParseNotes_NormalizedHash(t *testing.T) {
	parsed, _ := slides.ParseNotes(notesMD)
	if len(parsed[0].NormalizedHash) != 64 {
		t.Errorf("NormalizedHash len = %d, want 64", len(parsed[0].NormalizedHash))
	}
}

func TestParseNotes_HashOnCleanText(t *testing.T) {
	// Verify that the hash is computed on the clean text (markers stripped).
	// Two notes with identical words but different marker counts must have
	// the same clean text and thus the same hash.
	withPause := "## Slide 1\n\n<!-- source: seg-0001 -->\n\nHello world. Goodbye.\n"
	p1, _ := slides.ParseNotes(withPause)
	p2, _ := slides.ParseNotes(withPause)
	if p1[0].NormalizedHash != p2[0].NormalizedHash {
		t.Error("same input must produce same NormalizedHash (deterministic)")
	}
	if len(p1[0].NormalizedHash) != 64 {
		t.Errorf("NormalizedHash len = %d, want 64", len(p1[0].NormalizedHash))
	}
}

func TestParseNotes_InvalidTimestamp(t *testing.T) {
	bad := "## Slide 1\n\n<!-- source: seg-0001 -->\n<!-- approved-at: not-a-timestamp -->\n\nSome text.\n"
	_, errs := slides.ParseNotes(bad)
	if len(errs) == 0 {
		t.Error("expected parse error for invalid approved-at timestamp")
	}
}

func TestParseNotes_WithFixture(t *testing.T) {
	data, err := os.ReadFile("../../testdata/fixtures/speaker-notes.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, errs := slides.ParseNotes(string(data))
	if len(errs) != 0 {
		t.Errorf("fixture parse errors: %v", errs)
	}
	if len(parsed) != 3 {
		t.Errorf("fixture note count = %d, want 3", len(parsed))
	}
}

func TestParseNotes_InvalidFixture(t *testing.T) {
	data, err := os.ReadFile("../../testdata/fixtures/speaker-notes-invalid.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	_, errs := slides.ParseNotes(string(data))
	if len(errs) == 0 {
		t.Error("expected at least one parse error from invalid fixture")
	}
}

// ────────── Match ──────────

func TestMatch_Happy(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	ns, _ := slides.ParseNotes(notesMD)
	result := slides.Match(ss, ns, false)
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
	if len(result.Slides) != 2 {
		t.Errorf("matched slides = %d, want 2", len(result.Slides))
	}
}

func TestMatch_AllNotesPaired(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	ns, _ := slides.ParseNotes(notesMD)
	result := slides.Match(ss, ns, false)
	for _, ms := range result.Slides {
		if ms.Note == nil {
			t.Errorf("slide %d has no paired note", ms.Slide.Number)
		}
	}
}

func TestMatch_MissingNote(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	// Supply only one note for two slides.
	ns, _ := slides.ParseNotes("## Slide 1\n\n<!-- source: seg-0001 -->\n\nText.\n")
	result := slides.Match(ss, ns, false)
	if len(result.Errors) == 0 {
		t.Error("expected error for slide with no note")
	}
}

func TestMatch_OrphanNote(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	// Add a note for slide 99 which does not exist.
	ns, _ := slides.ParseNotes(notesMD + "\n## Slide 99\n\n<!-- source: seg-0001 -->\n\nExtra.\n")
	result := slides.Match(ss, ns, false)
	hasOrphanError := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "slide 99") {
			hasOrphanError = true
		}
	}
	if !hasOrphanError {
		t.Errorf("expected orphan-note error for slide 99; got errors: %v", result.Errors)
	}
}

func TestMatch_ApprovalRequired_Missing(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	ns, _ := slides.ParseNotes(notesMD)
	// Slide 2 note has no approval in notesMD.
	result := slides.Match(ss, ns, true)
	hasApprovalError := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "approval") || strings.Contains(e.Message, "approved-by") {
			hasApprovalError = true
		}
	}
	if !hasApprovalError {
		t.Errorf("expected approval error; got errors: %v", result.Errors)
	}
}

func TestMatch_ApprovalNotRequired(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	ns, _ := slides.ParseNotes(notesMD)
	// Slide 2 has no approval; with requireApproval=false this should not error.
	result := slides.Match(ss, ns, false)
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "approval") || strings.Contains(e.Message, "approved-by") {
			t.Errorf("unexpected approval error when not required: %v", e)
		}
	}
}

func TestMatch_MissingSourceIDs(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	noSource := "## Slide 1\n\nNo source refs here.\n\n## Slide 2\n\n<!-- source: seg-0002 -->\n\nText.\n"
	ns, _ := slides.ParseNotes(noSource)
	result := slides.Match(ss, ns, false)
	hasProvError := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "provenance") || strings.Contains(e.Message, "source") {
			hasProvError = true
		}
	}
	if !hasProvError {
		t.Errorf("expected provenance error for missing source IDs; got: %v", result.Errors)
	}
}

func TestMatch_CombinedHash(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	ns, _ := slides.ParseNotes(notesMD)
	r1 := slides.Match(ss, ns, false)
	r2 := slides.Match(ss, ns, false)
	if r1.CombinedHash == "" {
		t.Error("CombinedHash must not be empty for matched slides")
	}
	if r1.CombinedHash != r2.CombinedHash {
		t.Error("CombinedHash must be deterministic")
	}
}

func TestMatch_CombinedHashChanges(t *testing.T) {
	ss, _ := slides.ParseSlides(slidesMD)
	ns, _ := slides.ParseNotes(notesMD)
	r1 := slides.Match(ss, ns, false)

	modified := strings.Replace(slidesMD, "design paradigm", "altered text", 1)
	ss2, _ := slides.ParseSlides(modified)
	r2 := slides.Match(ss2, ns, false)
	if r1.CombinedHash == r2.CombinedHash {
		t.Error("CombinedHash must change when slide body changes")
	}
}

func TestMatch_WithFixtures(t *testing.T) {
	sData, err := os.ReadFile("../../testdata/fixtures/slides.md")
	if err != nil {
		t.Fatalf("read slides: %v", err)
	}
	nData, err := os.ReadFile("../../testdata/fixtures/speaker-notes.md")
	if err != nil {
		t.Fatalf("read notes: %v", err)
	}
	ss, _ := slides.ParseSlides(string(sData))
	ns, _ := slides.ParseNotes(string(nData))
	result := slides.Match(ss, ns, false)
	// Slide 2 has no approval; all others are approved. requireApproval=false.
	if len(result.Slides) != 3 {
		t.Errorf("fixture matched slides = %d, want 3", len(result.Slides))
	}
}
