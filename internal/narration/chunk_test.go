package narration_test

import (
	"strings"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/narration"
)

func singleInput(text string) []narration.Input {
	return []narration.Input{{SlideNumber: 1, Text: text, SourceIDs: []string{"seg-0001"}}}
}

// ────────── DefaultOptions ──────────

func TestDefaultOptions_MaxChars(t *testing.T) {
	opts := narration.DefaultOptions()
	if opts.MaxChars != 500 {
		t.Errorf("MaxChars = %d, want 500", opts.MaxChars)
	}
}

func TestDefaultOptions_HasProtectedPatterns(t *testing.T) {
	opts := narration.DefaultOptions()
	if len(opts.ProtectedPatterns) == 0 {
		t.Error("ProtectedPatterns must not be empty")
	}
}

// ────────── Split – basic splitting ──────────

func TestSplit_EmptyInput(t *testing.T) {
	result := narration.Split(singleInput(""), narration.DefaultOptions())
	if len(result.Chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(result.Chunks))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors for empty input, got %d", len(result.Errors))
	}
}

func TestSplit_WhitespaceOnlyInput(t *testing.T) {
	result := narration.Split(singleInput("   \n  "), narration.DefaultOptions())
	if len(result.Chunks) != 0 {
		t.Errorf("expected 0 chunks for whitespace-only input, got %d", len(result.Chunks))
	}
}

func TestSplit_SingleSentence(t *testing.T) {
	result := narration.Split(singleInput("Hello world."), narration.DefaultOptions())
	if len(result.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result.Chunks))
	}
	if result.Chunks[0].Text != "Hello world." {
		t.Errorf("chunk text = %q, want %q", result.Chunks[0].Text, "Hello world.")
	}
}

func TestSplit_MultipleSentences_NoLimitExceeded(t *testing.T) {
	text := "First sentence. Second sentence. Third sentence."
	opts := narration.DefaultOptions()
	opts.MaxChars = 500
	result := narration.Split(singleInput(text), opts)
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
	// All sentences fit in 500 chars; they should be combined into one chunk.
	if len(result.Chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	// Reconstruction must contain all original words.
	joined := ""
	for _, c := range result.Chunks {
		joined += c.Text + " "
	}
	for _, word := range []string{"First", "Second", "Third"} {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q missing from chunks", word)
		}
	}
}

func TestSplit_MaxCharsSplitsAtSentence(t *testing.T) {
	// Each sentence is ~30 chars; MaxChars=40 forces a split after the first.
	text := "Hello there world. Goodbye there now."
	opts := narration.DefaultOptions()
	opts.MaxChars = 20
	result := narration.Split(singleInput(text), opts)
	if len(result.Chunks) < 2 {
		t.Fatalf("expected ≥2 chunks with MaxChars=40, got %d: %v",
			len(result.Chunks), chunksText(result.Chunks))
	}
}

func TestSplit_ChunkNeverExceedsMaxChars(t *testing.T) {
	text := "Alpha. Beta. Gamma. Delta. Epsilon. Zeta. Eta. Theta. Iota. Kappa."
	opts := narration.DefaultOptions()
	opts.MaxChars = 30
	result := narration.Split(singleInput(text), opts)
	for _, c := range result.Chunks {
		if c.CharCount > opts.MaxChars {
			t.Errorf("chunk %q has %d chars, exceeds MaxChars=%d", c.Text, c.CharCount, opts.MaxChars)
		}
	}
}

func TestSplit_CharCountField(t *testing.T) {
	result := narration.Split(singleInput("Hello world."), narration.DefaultOptions())
	if len(result.Chunks) == 0 {
		t.Fatal("expected chunk")
	}
	c := result.Chunks[0]
	if c.CharCount != len(c.Text) {
		t.Errorf("CharCount=%d != len(Text)=%d", c.CharCount, len(c.Text))
	}
}

func TestSplit_ParagraphBreakIsMandatorySplit(t *testing.T) {
	text := "First paragraph sentence.\n\nSecond paragraph sentence."
	opts := narration.DefaultOptions()
	opts.MaxChars = 500 // large enough that without \n\n they'd be combined
	result := narration.Split(singleInput(text), opts)
	if len(result.Chunks) < 2 {
		t.Fatalf("paragraph break must force split; got %d chunks", len(result.Chunks))
	}
}

// ────────── Split – protected tokens ──────────

func TestSplit_URLNotSplitAtColon(t *testing.T) {
	// The colon in a URL must not trigger a clause split.
	text := "Visit https://example.com/path for details. Then continue."
	opts := narration.DefaultOptions()
	result := narration.Split(singleInput(text), opts)
	for _, c := range result.Chunks {
		// No chunk should contain just "https" or break mid-URL.
		if strings.HasSuffix(c.Text, "https") || strings.HasSuffix(c.Text, "http") {
			t.Errorf("URL appears to be split in chunk: %q", c.Text)
		}
	}
}

func TestSplit_InlineCodeNotSplit(t *testing.T) {
	// The period inside backtick code must not trigger a sentence split.
	text := "Call `foo.bar()` to initialize. Then call `baz()`."
	opts := narration.DefaultOptions()
	opts.MaxChars = 500
	result := narration.Split(singleInput(text), opts)
	// Both backtick regions must appear intact in some chunk.
	allText := strings.Join(chunksText(result.Chunks), " ")
	if !strings.Contains(allText, "`foo.bar()`") {
		t.Errorf("protected token `foo.bar()` was split; chunks: %v", chunksText(result.Chunks))
	}
	if !strings.Contains(allText, "`baz()`") {
		t.Errorf("protected token `baz()` was split; chunks: %v", chunksText(result.Chunks))
	}
}

func TestSplit_VersionStringNotSplit(t *testing.T) {
	// HTTP/2 must be treated as a protected token.
	text := "The server uses HTTP/2 for multiplexing. It also supports TLS/1.3."
	opts := narration.DefaultOptions()
	result := narration.Split(singleInput(text), opts)
	allText := strings.Join(chunksText(result.Chunks), " ")
	if !strings.Contains(allText, "HTTP/2") {
		t.Errorf("protected token HTTP/2 was split or lost; got: %s", allText)
	}
}

// ────────── Split – character-limit rejection ──────────

func TestSplit_SingleOversizedTokenIsError(t *testing.T) {
	// A single "word" (protected token) that exceeds MaxChars cannot be split.
	bigToken := "`" + strings.Repeat("x", 600) + "`"
	text := "Prefix. " + bigToken + " Suffix."
	opts := narration.DefaultOptions()
	opts.MaxChars = 100
	result := narration.Split(singleInput(text), opts)
	if len(result.Errors) == 0 {
		t.Error("expected ChunkError for oversized protected token, got none")
	}
}

func TestSplit_ErrorContainsSlideNumber(t *testing.T) {
	bigToken := "`" + strings.Repeat("x", 600) + "`"
	inp := narration.Input{SlideNumber: 5, Text: bigToken, SourceIDs: []string{"seg-0001"}}
	opts := narration.DefaultOptions()
	opts.MaxChars = 100
	result := narration.Split([]narration.Input{inp}, opts)
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	if result.Errors[0].SlideNumber != 5 {
		t.Errorf("error SlideNumber = %d, want 5", result.Errors[0].SlideNumber)
	}
}

func TestSplit_NoLimitAcceptsLongChunks(t *testing.T) {
	long := strings.Repeat("word. ", 200)
	opts := narration.DefaultOptions()
	opts.MaxChars = 0 // no limit
	result := narration.Split(singleInput(long), opts)
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors with MaxChars=0, got: %v", result.Errors)
	}
}

// ────────── Split – determinism and IDs ──────────

func TestSplit_Deterministic(t *testing.T) {
	text := "Hello world. How are you? Fine thanks."
	opts := narration.DefaultOptions()
	r1 := narration.Split(singleInput(text), opts)
	r2 := narration.Split(singleInput(text), opts)
	if len(r1.Chunks) != len(r2.Chunks) {
		t.Fatalf("chunk count differs: %d vs %d", len(r1.Chunks), len(r2.Chunks))
	}
	for i := range r1.Chunks {
		if r1.Chunks[i].ID != r2.Chunks[i].ID {
			t.Errorf("chunk[%d].ID differs: %q vs %q", i, r1.Chunks[i].ID, r2.Chunks[i].ID)
		}
		if r1.Chunks[i].Seed != r2.Chunks[i].Seed {
			t.Errorf("chunk[%d].Seed differs", i)
		}
	}
}

func TestSplit_IDLength(t *testing.T) {
	result := narration.Split(singleInput("Hello world."), narration.DefaultOptions())
	if len(result.Chunks) == 0 {
		t.Fatal("expected chunk")
	}
	if len(result.Chunks[0].ID) != 16 {
		t.Errorf("chunk ID len = %d, want 16", len(result.Chunks[0].ID))
	}
}

func TestSplit_GlobalSeedAffectsSeed(t *testing.T) {
	text := "Hello world."
	opts1 := narration.DefaultOptions()
	opts1.GlobalSeed = 0
	opts2 := narration.DefaultOptions()
	opts2.GlobalSeed = 42
	r1 := narration.Split(singleInput(text), opts1)
	r2 := narration.Split(singleInput(text), opts2)
	if len(r1.Chunks) == 0 || len(r2.Chunks) == 0 {
		t.Fatal("expected chunks")
	}
	if r1.Chunks[0].Seed == r2.Chunks[0].Seed {
		t.Error("different GlobalSeed must produce different chunk Seeds")
	}
}

func TestSplit_IDStableWithSameSlideAndIndex(t *testing.T) {
	// ID depends only on SlideNumber and OrderIndex, not text content.
	r1 := narration.Split(singleInput("Hello world."), narration.DefaultOptions())
	r2 := narration.Split(singleInput("Different text."), narration.DefaultOptions())
	if len(r1.Chunks) == 0 || len(r2.Chunks) == 0 {
		t.Fatal("expected chunks")
	}
	if r1.Chunks[0].ID != r2.Chunks[0].ID {
		t.Errorf("ID should be the same for slide=1 index=0 regardless of text: %q vs %q",
			r1.Chunks[0].ID, r2.Chunks[0].ID)
	}
}

// ────────── Split – source provenance ──────────

func TestSplit_SourceIDsPreserved(t *testing.T) {
	inp := narration.Input{
		SlideNumber: 2,
		Text:        "First. Second.",
		SourceIDs:   []string{"seg-0002", "seg-0003"},
	}
	result := narration.Split([]narration.Input{inp}, narration.DefaultOptions())
	for _, c := range result.Chunks {
		if len(c.SourceIDs) != 2 {
			t.Errorf("chunk.SourceIDs = %v, want [seg-0002 seg-0003]", c.SourceIDs)
		}
	}
}

func TestSplit_OrderIndexIsGlobal(t *testing.T) {
	inputs := []narration.Input{
		{SlideNumber: 1, Text: "Alpha. Beta.", SourceIDs: []string{"seg-0001"}},
		{SlideNumber: 2, Text: "Gamma. Delta.", SourceIDs: []string{"seg-0002"}},
	}
	opts := narration.DefaultOptions()
	opts.MaxChars = 10 // force splits
	result := narration.Split(inputs, opts)
	for i, c := range result.Chunks {
		if c.OrderIndex != i {
			t.Errorf("chunk[%d].OrderIndex = %d, want %d", i, c.OrderIndex, i)
		}
	}
}

// ────────── Schedule ──────────

func TestSchedule_Order(t *testing.T) {
	text := "Alpha. Beta. Gamma. Delta."
	opts := narration.DefaultOptions()
	opts.MaxChars = 10
	result := narration.Split(singleInput(text), opts)
	sched := narration.Schedule(result)

	for i := 1; i < len(sched); i++ {
		if sched[i].OrderIndex <= sched[i-1].OrderIndex {
			t.Errorf("schedule not sorted: [%d].OrderIndex=%d >= [%d].OrderIndex=%d",
				i-1, sched[i-1].OrderIndex, i, sched[i].OrderIndex)
		}
	}
}

func TestSchedule_PredecessorSuccessorLinks(t *testing.T) {
	text := "Alpha. Beta. Gamma."
	opts := narration.DefaultOptions()
	opts.MaxChars = 10
	result := narration.Split(singleInput(text), opts)
	sched := narration.Schedule(result)
	if len(sched) < 2 {
		t.Skip("need ≥2 chunks to test links")
	}

	// First chunk has no predecessor.
	if sched[0].PredecessorID != "" {
		t.Errorf("first chunk PredecessorID = %q, want empty", sched[0].PredecessorID)
	}
	// Last chunk has no successor.
	last := sched[len(sched)-1]
	if last.SuccessorID != "" {
		t.Errorf("last chunk SuccessorID = %q, want empty", last.SuccessorID)
	}
	// Middle chunks link correctly.
	for i := 1; i < len(sched)-1; i++ {
		if sched[i].PredecessorID != sched[i-1].ID {
			t.Errorf("sched[%d].PredecessorID = %q, want %q", i, sched[i].PredecessorID, sched[i-1].ID)
		}
		if sched[i].SuccessorID != sched[i+1].ID {
			t.Errorf("sched[%d].SuccessorID = %q, want %q", i, sched[i].SuccessorID, sched[i+1].ID)
		}
	}
}

func TestSchedule_EmptyResult(t *testing.T) {
	sched := narration.Schedule(narration.Result{})
	if sched == nil {
		t.Error("Schedule on empty result must return non-nil (empty) slice")
	}
	if len(sched) != 0 {
		t.Errorf("len = %d, want 0", len(sched))
	}
}

func TestSchedule_Deterministic(t *testing.T) {
	text := "Alpha. Beta. Gamma."
	opts := narration.DefaultOptions()
	opts.MaxChars = 10
	r := narration.Split(singleInput(text), opts)
	s1 := narration.Schedule(r)
	s2 := narration.Schedule(r)
	if len(s1) != len(s2) {
		t.Fatalf("schedule length differs: %d vs %d", len(s1), len(s2))
	}
	for i := range s1 {
		if s1[i].ID != s2[i].ID {
			t.Errorf("schedule[%d].ID differs", i)
		}
	}
}

// ────────── Helpers ──────────

func chunksText(chunks []narration.Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Text
	}
	return out
}
