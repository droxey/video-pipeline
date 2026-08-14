package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "parser", name))
	if err != nil {
		t.Fatalf("readFixture %s: %v", name, err)
	}
	return data
}

// errCodes returns just the Code field of each ParseError for compact assertions.
func errCodes(errs []ParseError) []string {
	codes := make([]string, len(errs))
	for i, e := range errs {
		codes[i] = e.Code
	}
	return codes
}

// hasCode reports whether any error in errs has the given code.
func hasCode(errs []ParseError, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}

// ─── Happy path ───────────────────────────────────────────────────────────────

func TestParseBoth_Valid(t *testing.T) {
	slidesData := readFixture(t, "valid_slides.md")
	notesData := readFixture(t, "valid_notes.md")

	result, errs := ParseBoth(slidesData, notesData)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if result == nil {
		t.Fatal("result must not be nil")
	}

	if len(result.Slides) != 3 {
		t.Fatalf("expected 3 slides, got %d", len(result.Slides))
	}

	// Slide ordering must follow source order (sorted by number).
	if result.Slides[0].Number != 1 {
		t.Errorf("first slide should be 1, got %d", result.Slides[0].Number)
	}
	if result.Slides[2].Number != 3 {
		t.Errorf("third slide should be 3, got %d", result.Slides[2].Number)
	}

	// Check metadata on slide 1.
	s1 := result.Slides[0]
	if s1.Title != "Introduction to Go" {
		t.Errorf("slide 1 title: got %q", s1.Title)
	}
	if s1.DurationSeconds != 30.5 {
		t.Errorf("slide 1 duration: got %g", s1.DurationSeconds)
	}
	if len(s1.SFX) != 1 || s1.SFX[0].Name != "keyboard-typing" {
		t.Errorf("slide 1 SFX: got %v", s1.SFX)
	}
	if len(s1.Dialogue) != 2 {
		t.Errorf("slide 1 dialogue entries: got %d", len(s1.Dialogue))
	}
	if s1.Body == "" {
		t.Error("slide 1 body must not be empty")
	}

	// Check multiple SFX markers on slide 3.
	s3 := result.Slides[2]
	if len(s3.SFX) != 2 {
		t.Errorf("slide 3 should have 2 SFX markers, got %d", len(s3.SFX))
	}

	// Dialogue approval fields must be populated.
	d := s1.Dialogue[0]
	if d.Speaker != "Narrator" {
		t.Errorf("dialogue speaker: got %q", d.Speaker)
	}
	if d.Provenance != ProvenanceHuman {
		t.Errorf("dialogue provenance: got %q", d.Provenance)
	}
	if d.Approval.ApprovedBy != "alice@nebula.com" {
		t.Errorf("approved-by: got %q", d.Approval.ApprovedBy)
	}
	want := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	if !d.Approval.ApprovedAt.Equal(want) {
		t.Errorf("approved-at: got %v", d.Approval.ApprovedAt)
	}

	// Content hashes must be non-empty and stable.
	if s1.ContentHash == "" {
		t.Error("slide 1 content hash must not be empty")
	}

	// Re-parsing identical input must produce identical hashes.
	result2, errs2 := ParseBoth(slidesData, notesData)
	if len(errs2) != 0 {
		t.Fatalf("second parse produced errors: %v", errs2)
	}
	if result.Slides[0].ContentHash != result2.Slides[0].ContentHash {
		t.Error("content hash is not deterministic across identical parses")
	}
	if result.SlidesHash != result2.SlidesHash {
		t.Error("slides file hash is not deterministic")
	}
	if result.NotesHash != result2.NotesHash {
		t.Error("notes file hash is not deterministic")
	}
}

// ─── Duplicate slide numbers ──────────────────────────────────────────────────

func TestParseSlides_DupSlide(t *testing.T) {
	data := readFixture(t, "dup_slide_slides.md")
	slides, errs := parseSlides(data)
	if !hasCode(errs, CodeDupSlide) {
		t.Fatalf("expected %s, got codes %v", CodeDupSlide, errCodes(errs))
	}
	// The duplicate should be dropped; only 2 unique slides remain.
	if len(slides) != 2 {
		t.Errorf("expected 2 unique slides after dedup, got %d", len(slides))
	}
}

func TestParseNotes_DupSlide(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\nNarrator: Hi.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n\n## Slide 1: Intro\n\nNarrator: Again.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n")
	_, errs := parseNotes(raw)
	if !hasCode(errs, CodeDupSlide) {
		t.Fatalf("expected %s, got %v", CodeDupSlide, errCodes(errs))
	}
}

// ─── Missing slides ───────────────────────────────────────────────────────────

func TestParseBoth_MissingSlideInNotes(t *testing.T) {
	// slides.md has slides 1, 2, 3; notes only has 1 and 3 → slide 2 missing.
	slidesData := readFixture(t, "valid_slides.md")
	notesData := readFixture(t, "missing_notes.md")

	_, errs := ParseBoth(slidesData, notesData)
	if !hasCode(errs, CodeMissingSlide) {
		t.Fatalf("expected %s, got %v", CodeMissingSlide, errCodes(errs))
	}
}

func TestParseBoth_MissingSlideInSlides(t *testing.T) {
	// notes has a slide that slides.md does not.
	slidesData := []byte("## Slide 1: Only Slide\n\n<!-- duration_seconds: 10.0 -->\n\nBody.\n")
	notesData := []byte("## Slide 1: Only Slide\n\nNarrator: Hello.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n\n## Slide 2: Extra\n\nNarrator: Extra.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n")

	_, errs := ParseBoth(slidesData, notesData)
	if !hasCode(errs, CodeMissingSlide) {
		t.Fatalf("expected %s, got %v", CodeMissingSlide, errCodes(errs))
	}
}

// ─── Title mismatch ───────────────────────────────────────────────────────────

func TestParseBoth_TitleMismatch(t *testing.T) {
	slidesData := []byte("## Slide 1: The Real Title\n\nBody.\n")
	notesData := []byte("## Slide 1: A Different Title\n\nNarrator: Hi.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n")

	result, errs := ParseBoth(slidesData, notesData)
	if !hasCode(errs, CodeTitleMismatch) {
		t.Fatalf("expected %s, got %v", CodeTitleMismatch, errCodes(errs))
	}
	// slides.md title must be authoritative in the output.
	if len(result.Slides) > 0 && result.Slides[0].Title != "The Real Title" {
		t.Errorf("expected slides.md title to be authoritative, got %q", result.Slides[0].Title)
	}
}

// ─── Missing approval metadata ────────────────────────────────────────────────

func TestParseNotes_MissingApproval(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\nNarrator: No approval at all.\n")
	_, errs := parseNotes(raw)
	if !hasCode(errs, CodeNoApproval) {
		t.Fatalf("expected %s, got %v", CodeNoApproval, errCodes(errs))
	}
}

func TestParseNotes_PartialApproval(t *testing.T) {
	// Only provenance provided; approved-by and approved-at missing.
	raw := []byte("## Slide 1: Intro\n\nNarrator: Partial.\n<!-- provenance: human -->\n")
	_, errs := parseNotes(raw)
	if !hasCode(errs, CodeNoApproval) {
		t.Fatalf("expected %s, got %v", CodeNoApproval, errCodes(errs))
	}
}

// ─── Unknown metadata ─────────────────────────────────────────────────────────

func TestParseSlides_UnknownMeta(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\n<!-- unknown-key: val -->\n\nBody.\n")
	_, errs := parseSlides(raw)
	if !hasCode(errs, CodeUnknownMeta) {
		t.Fatalf("expected %s, got %v", CodeUnknownMeta, errCodes(errs))
	}
}

func TestParseNotes_UnknownMeta(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\nNarrator: Hi.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n<!-- weird-key: yep -->\n")
	_, errs := parseNotes(raw)
	if !hasCode(errs, CodeUnknownMeta) {
		t.Fatalf("expected %s, got %v", CodeUnknownMeta, errCodes(errs))
	}
}

// ─── Bad provenance ───────────────────────────────────────────────────────────

func TestParseNotes_BadProvenance(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\nNarrator: Hi.\n<!-- provenance: robot -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n")
	_, errs := parseNotes(raw)
	if !hasCode(errs, CodeBadProvenance) {
		t.Fatalf("expected %s, got %v", CodeBadProvenance, errCodes(errs))
	}
}

// ─── Bad timestamp ────────────────────────────────────────────────────────────

func TestParseNotes_BadTimestamp(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\nNarrator: Hi.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: yesterday -->\n")
	_, errs := parseNotes(raw)
	if !hasCode(errs, CodeBadTimestamp) {
		t.Fatalf("expected %s, got %v", CodeBadTimestamp, errCodes(errs))
	}
}

// ─── Malformed duration ───────────────────────────────────────────────────────

func TestParseSlides_BadDuration(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\n<!-- duration_seconds: banana -->\n\nBody.\n")
	_, errs := parseSlides(raw)
	if !hasCode(errs, CodeMalformedMeta) {
		t.Fatalf("expected %s, got %v", CodeMalformedMeta, errCodes(errs))
	}
}

func TestParseSlides_NegativeDuration(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\n<!-- duration_seconds: -5 -->\n\nBody.\n")
	_, errs := parseSlides(raw)
	if !hasCode(errs, CodeMalformedMeta) {
		t.Fatalf("expected %s, got %v", CodeMalformedMeta, errCodes(errs))
	}
}

// ─── Malformed dialogue ───────────────────────────────────────────────────────

func TestParseNotes_MalformedDialogue(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\nThis line has no speaker prefix.\n")
	_, errs := parseNotes(raw)
	if !hasCode(errs, CodeMalformedDialogue) {
		t.Fatalf("expected %s, got %v", CodeMalformedDialogue, errCodes(errs))
	}
}

// ─── SFX markers ─────────────────────────────────────────────────────────────

func TestParseSlides_SFXMarkers(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\n<!-- sfx: whoosh -->\n<!-- sfx: chime -->\n\nBody.\n")
	slides, errs := parseSlides(raw)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(slides) != 1 {
		t.Fatalf("expected 1 slide, got %d", len(slides))
	}
	if len(slides[0].SFX) != 2 {
		t.Fatalf("expected 2 SFX markers, got %d", len(slides[0].SFX))
	}
	if slides[0].SFX[0].Name != "whoosh" {
		t.Errorf("SFX[0]: got %q", slides[0].SFX[0].Name)
	}
	if slides[0].SFX[1].Name != "chime" {
		t.Errorf("SFX[1]: got %q", slides[0].SFX[1].Name)
	}
}

// ─── Empty SFX value ─────────────────────────────────────────────────────────

func TestParseSlides_EmptySFX(t *testing.T) {
	raw := []byte("## Slide 1: Intro\n\n<!-- sfx:  -->\n\nBody.\n")
	_, errs := parseSlides(raw)
	if !hasCode(errs, CodeMalformedMeta) {
		t.Fatalf("expected %s for empty sfx, got %v", CodeMalformedMeta, errCodes(errs))
	}
}

// ─── Content hash determinism ─────────────────────────────────────────────────

func TestSlideContentHash_Deterministic(t *testing.T) {
	ps := ParsedSlide{
		Number:          1,
		Title:           "Test",
		Body:            "body",
		DurationSeconds: 10,
		SFX:             []SFXMarker{{Name: "click"}},
		Dialogue: []DialogueEntry{
			{
				Speaker:    "Narrator",
				Text:       "Hello.",
				Provenance: ProvenanceHuman,
				Approval: ApprovalMeta{
					ApprovedBy: "alice",
					ApprovedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	h1 := slideContentHash(ps)
	h2 := slideContentHash(ps)
	if h1 != h2 {
		t.Error("slideContentHash is not deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got %d chars", len(h1))
	}

	// Changing any field must change the hash.
	ps2 := ps
	ps2.Title = "Different"
	if slideContentHash(ps2) == h1 {
		t.Error("hash did not change after title mutation")
	}
}

// ─── WriteOutput ──────────────────────────────────────────────────────────────

func TestWriteOutput(t *testing.T) {
	slidesData := readFixture(t, "valid_slides.md")
	notesData := readFixture(t, "valid_notes.md")

	result, parseErrs := ParseBoth(slidesData, notesData)
	if len(parseErrs) != 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}

	dir := t.TempDir()
	if err := WriteOutput(dir, result, parseErrs); err != nil {
		t.Fatalf("WriteOutput: %v", err)
	}

	// normalized.json must exist and decode correctly.
	normPath := filepath.Join(dir, "normalized.json")
	raw, err := os.ReadFile(normPath)
	if err != nil {
		t.Fatalf("read normalized.json: %v", err)
	}
	var got ParseResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal normalized.json: %v", err)
	}
	if len(got.Slides) != len(result.Slides) {
		t.Errorf("normalized.json: slide count mismatch: want %d got %d",
			len(result.Slides), len(got.Slides))
	}

	// parser-errors.json must exist and decode as an empty array.
	errPath := filepath.Join(dir, "parser-errors.json")
	raw, err = os.ReadFile(errPath)
	if err != nil {
		t.Fatalf("read parser-errors.json: %v", err)
	}
	var gotErrs []ParseError
	if err := json.Unmarshal(raw, &gotErrs); err != nil {
		t.Fatalf("unmarshal parser-errors.json: %v", err)
	}
	if len(gotErrs) != 0 {
		t.Errorf("expected empty error array, got %v", gotErrs)
	}
}

func TestWriteOutput_WithErrors(t *testing.T) {
	slidesData := []byte("## Slide 1: Intro\n\nBody.\n")
	notesData := []byte("## Slide 1: Intro\n\nNarrator: Hi.\n<!-- provenance: human -->\n<!-- approved-by: x -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n")

	result, parseErrs := ParseBoth(slidesData, notesData)

	dir := t.TempDir()
	if err := WriteOutput(dir, result, parseErrs); err != nil {
		t.Fatalf("WriteOutput: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "parser-errors.json"))
	if err != nil {
		t.Fatalf("read parser-errors.json: %v", err)
	}
	var gotErrs []ParseError
	if err := json.Unmarshal(raw, &gotErrs); err != nil {
		t.Fatalf("unmarshal parser-errors.json: %v", err)
	}
	// This valid input should produce no errors; test that the file is [] not null.
	if string(raw[:2]) == "nu" {
		t.Error("parser-errors.json must be [] not null when there are no errors")
	}
}

// ─── Edge cases ───────────────────────────────────────────────────────────────

func TestParseBoth_EmptyInputs(t *testing.T) {
	result, errs := ParseBoth([]byte(""), []byte(""))
	if result == nil {
		t.Fatal("result must not be nil for empty inputs")
	}
	if len(result.Slides) != 0 {
		t.Errorf("expected 0 slides, got %d", len(result.Slides))
	}
	if len(errs) != 0 {
		t.Errorf("unexpected errors for empty inputs: %v", errs)
	}
}

func TestParseBoth_MultipleProvenanceValues(t *testing.T) {
	slidesData := []byte("## Slide 1: Intro\n\nBody.\n")
	notesData := []byte("## Slide 1: Intro\n\nNarrator: Human text.\n<!-- provenance: human -->\n<!-- approved-by: alice -->\n<!-- approved-at: 2024-01-01T00:00:00Z -->\n\nNarrator: AI text.\n<!-- provenance: ai-generated -->\n<!-- approved-by: bob -->\n<!-- approved-at: 2024-01-01T01:00:00Z -->\n")

	result, errs := ParseBoth(slidesData, notesData)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(result.Slides[0].Dialogue) != 2 {
		t.Errorf("expected 2 dialogue entries, got %d", len(result.Slides[0].Dialogue))
	}
	if result.Slides[0].Dialogue[0].Provenance != ProvenanceHuman {
		t.Errorf("first entry: got provenance %q", result.Slides[0].Dialogue[0].Provenance)
	}
	if result.Slides[0].Dialogue[1].Provenance != ProvenanceAIGenerated {
		t.Errorf("second entry: got provenance %q", result.Slides[0].Dialogue[1].Provenance)
	}
}

func TestParseError_ErrorMethod(t *testing.T) {
	e := ParseError{File: "slides.md", Line: 5, Slide: 1, Code: CodeDupSlide, Message: "dup"}
	s := e.Error()
	if s == "" {
		t.Error("Error() must not be empty")
	}

	e2 := ParseError{File: "slides.md", Code: CodeMissingSlide, Message: "missing"}
	s2 := e2.Error()
	if s2 == "" {
		t.Error("Error() without line must not be empty")
	}
}
