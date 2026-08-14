package subtitles_test

import (
	"strings"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/providers"
	"github.com/nebula/course-video-pipeline/internal/subtitles"
)

// ────────── helpers ──────────

func wordsAlignment(words ...string) providers.Alignment {
	timings := make([]providers.WordTiming, len(words))
	for i, w := range words {
		timings[i] = providers.WordTiming{
			Word:  w,
			Start: float64(i),
			End:   float64(i + 1),
		}
	}
	return providers.Alignment{Words: timings}
}

// ────────── BuildTrack ──────────

func TestBuildTrack_Empty_NoCues(t *testing.T) {
	al := providers.Alignment{}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	if len(track.Cues) != 0 {
		t.Errorf("expected 0 cues for empty alignment, got %d", len(track.Cues))
	}
}

func TestBuildTrack_SingleWord(t *testing.T) {
	al := wordsAlignment("hello")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	if len(track.Cues) != 1 {
		t.Fatalf("expected 1 cue, got %d", len(track.Cues))
	}
	if track.Cues[0].Text != "hello" {
		t.Errorf("cue text = %q, want hello", track.Cues[0].Text)
	}
}

func TestBuildTrack_GroupsWordsPerCue(t *testing.T) {
	al := wordsAlignment("one", "two", "three", "four", "five", "six", "seven", "eight")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 3})
	// 8 words / 3 per cue = ceil(8/3) = 3 cues
	if len(track.Cues) != 3 {
		t.Errorf("cue count = %d, want 3", len(track.Cues))
	}
	if track.Cues[0].Text != "one two three" {
		t.Errorf("cue[0].Text = %q, want %q", track.Cues[0].Text, "one two three")
	}
}

func TestBuildTrack_DefaultWordsPerCue(t *testing.T) {
	words := make([]string, subtitles.DefaultWordsPerCue+1)
	for i := range words {
		words[i] = "w"
	}
	al := wordsAlignment(words...)
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	if len(track.Cues) < 2 {
		t.Errorf("expected ≥2 cues with %d words and default WordsPerCue=%d, got %d",
			len(words), subtitles.DefaultWordsPerCue, len(track.Cues))
	}
}

func TestBuildTrack_CueIndex_StartsAt1(t *testing.T) {
	al := wordsAlignment("a", "b", "c")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 1})
	for i, c := range track.Cues {
		if c.Index != i+1 {
			t.Errorf("cue[%d].Index = %d, want %d", i, c.Index, i+1)
		}
	}
}

func TestBuildTrack_CueTimings(t *testing.T) {
	al := wordsAlignment("hello", "world") // start=0,1 end=1,2
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 2})
	if len(track.Cues) != 1 {
		t.Fatalf("expected 1 cue, got %d", len(track.Cues))
	}
	cue := track.Cues[0]
	if cue.Start != 0.0 {
		t.Errorf("Start = %f, want 0.0", cue.Start)
	}
	if cue.End != 2.0 {
		t.Errorf("End = %f, want 2.0", cue.End)
	}
}

func TestBuildTrack_DefaultLanguage(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	if track.Language != "en" {
		t.Errorf("Language = %q, want en", track.Language)
	}
}

func TestBuildTrack_CustomLanguage(t *testing.T) {
	al := wordsAlignment("hola")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{Language: "es"})
	if track.Language != "es" {
		t.Errorf("Language = %q, want es", track.Language)
	}
}

func TestBuildTrack_DefaultFormat(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	if track.Format != "vtt" {
		t.Errorf("Format = %q, want vtt", track.Format)
	}
}

func TestBuildTrack_CustomFormat(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{Format: "srt"})
	if track.Format != "srt" {
		t.Errorf("Format = %q, want srt", track.Format)
	}
}

// ────────── FormatVTT ──────────

func TestFormatVTT_Header(t *testing.T) {
	al := wordsAlignment("hello")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	vtt := subtitles.FormatVTT(track)
	if !strings.HasPrefix(vtt, "WEBVTT\n") {
		t.Errorf("VTT must start with WEBVTT header; got: %q", vtt[:min(20, len(vtt))])
	}
}

func TestFormatVTT_ContainsCueIndex(t *testing.T) {
	al := wordsAlignment("hello", "world")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 1})
	vtt := subtitles.FormatVTT(track)
	if !strings.Contains(vtt, "1\n") {
		t.Errorf("VTT must contain cue index 1; got: %s", vtt)
	}
	if !strings.Contains(vtt, "2\n") {
		t.Errorf("VTT must contain cue index 2; got: %s", vtt)
	}
}

func TestFormatVTT_TimestampArrow(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	vtt := subtitles.FormatVTT(track)
	if !strings.Contains(vtt, " --> ") {
		t.Errorf("VTT must contain ' --> ' timestamp separator; got: %s", vtt)
	}
}

func TestFormatVTT_VTTTimestampFormat(t *testing.T) {
	// Cue starting at 0s should produce 00:00:00.000
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	vtt := subtitles.FormatVTT(track)
	if !strings.Contains(vtt, "00:00:00.000") {
		t.Errorf("VTT timestamp must use HH:MM:SS.mmm format; got: %s", vtt)
	}
}

func TestFormatVTT_CueText(t *testing.T) {
	al := wordsAlignment("hello", "world")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 2})
	vtt := subtitles.FormatVTT(track)
	if !strings.Contains(vtt, "hello world") {
		t.Errorf("VTT must contain cue text %q; got: %s", "hello world", vtt)
	}
}

func TestFormatVTT_EndsWithNewline(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	vtt := subtitles.FormatVTT(track)
	if !strings.HasSuffix(vtt, "\n") {
		t.Error("VTT output must end with a newline")
	}
}

func TestFormatVTT_LargeTimestamp(t *testing.T) {
	// Start = 3661.5s → 01:01:01.500
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "x", Start: 3661.5, End: 3662.0},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	vtt := subtitles.FormatVTT(track)
	if !strings.Contains(vtt, "01:01:01.500") {
		t.Errorf("VTT large timestamp wrong; got: %s", vtt)
	}
}

// ────────── FormatSRT ──────────

func TestFormatSRT_ContainsCueIndex(t *testing.T) {
	al := wordsAlignment("hello")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	srt := subtitles.FormatSRT(track)
	if !strings.Contains(srt, "1\n") {
		t.Errorf("SRT must contain cue index 1; got: %s", srt)
	}
}

func TestFormatSRT_TimestampArrowComma(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	srt := subtitles.FormatSRT(track)
	// SRT uses comma millisecond separator: HH:MM:SS,mmm
	if !strings.Contains(srt, "00:00:00,000") {
		t.Errorf("SRT must use HH:MM:SS,mmm timestamp format; got: %s", srt)
	}
}

func TestFormatSRT_NoVTTHeader(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{Format: "srt"})
	srt := subtitles.FormatSRT(track)
	if strings.HasPrefix(srt, "WEBVTT") {
		t.Error("SRT must not start with WEBVTT header")
	}
}

func TestFormatSRT_EndsWithNewline(t *testing.T) {
	al := wordsAlignment("hi")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	srt := subtitles.FormatSRT(track)
	if !strings.HasSuffix(srt, "\n") {
		t.Error("SRT output must end with a newline")
	}
}

func TestFormatSRT_CueText(t *testing.T) {
	al := wordsAlignment("hello", "world")
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 2, Format: "srt"})
	srt := subtitles.FormatSRT(track)
	if !strings.Contains(srt, "hello world") {
		t.Errorf("SRT must contain cue text; got: %s", srt)
	}
}

// min is a Go 1.22-compatible helper (built-in min only in 1.21+).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ────────── Unicode ──────────

func TestFormatVTT_Unicode_Words(t *testing.T) {
	// Words with accented characters, CJK, and emoji.
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "héllo", Start: 0.0, End: 1.0},
			{Word: "wörld", Start: 1.0, End: 2.0},
			{Word: "中文", Start: 2.0, End: 3.0},
			{Word: "🎉", Start: 3.0, End: 4.0},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 4})
	vtt := subtitles.FormatVTT(track)
	for _, word := range []string{"héllo", "wörld", "中文", "🎉"} {
		if !strings.Contains(vtt, word) {
			t.Errorf("VTT must contain Unicode word %q; got:\n%s", word, vtt)
		}
	}
}

func TestFormatSRT_Unicode_Words(t *testing.T) {
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "café", Start: 0.0, End: 1.0},
			{Word: "naïve", Start: 1.0, End: 2.0},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 2, Format: "srt"})
	srt := subtitles.FormatSRT(track)
	if !strings.Contains(srt, "café") || !strings.Contains(srt, "naïve") {
		t.Errorf("SRT must preserve Unicode word text; got:\n%s", srt)
	}
}

// ────────── Punctuation in word text ──────────

func TestFormatVTT_Punctuation_InWords(t *testing.T) {
	// Alignment may return words with punctuation attached.
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "hello,", Start: 0.0, End: 1.0},
			{Word: "world!", Start: 1.0, End: 2.0},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 2})
	vtt := subtitles.FormatVTT(track)
	if !strings.Contains(vtt, "hello, world!") {
		t.Errorf("VTT must preserve punctuation in word text; got:\n%s", vtt)
	}
}

func TestBuildTrack_EmptyWordText_Included(t *testing.T) {
	// BuildTrack does not filter empty strings from alignment output.
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "real", Start: 0.0, End: 1.0},
			{Word: "", Start: 1.0, End: 1.5},
			{Word: "word", Start: 1.5, End: 2.5},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 3})
	if len(track.Cues) != 1 {
		t.Fatalf("expected 1 cue, got %d", len(track.Cues))
	}
	if !strings.Contains(track.Cues[0].Text, "real") {
		t.Error("cue must contain 'real'")
	}
}

// ────────── Overlapping word timings ──────────

func TestBuildTrack_OverlappingTimings_DoesNotPanic(t *testing.T) {
	// Words with overlapping timings (End[i] > Start[i+1]).
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "alpha", Start: 0.0, End: 1.5},
			{Word: "beta", Start: 1.0, End: 2.5}, // overlaps alpha
			{Word: "gamma", Start: 2.0, End: 3.5},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 1})
	// Must not panic; must produce 3 cues.
	if len(track.Cues) != 3 {
		t.Errorf("expected 3 cues for 3 words, got %d", len(track.Cues))
	}
}

func TestBuildTrack_OverlappingTimings_PreservesWordBoundaries(t *testing.T) {
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "one", Start: 0.0, End: 2.0},
			{Word: "two", Start: 1.0, End: 3.0},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{WordsPerCue: 1})
	if track.Cues[0].Start != 0.0 || track.Cues[0].End != 2.0 {
		t.Errorf("cue[0] = [%.1f, %.1f], want [0.0, 2.0]", track.Cues[0].Start, track.Cues[0].End)
	}
	if track.Cues[1].Start != 1.0 || track.Cues[1].End != 3.0 {
		t.Errorf("cue[1] = [%.1f, %.1f], want [1.0, 3.0]", track.Cues[1].Start, track.Cues[1].End)
	}
}

// ────────── Millisecond rounding (truncation) ──────────

func TestFormatVTT_Rounding_TruncatesMilliseconds(t *testing.T) {
	// 0.9999 seconds → should truncate to 00:00:00.999 not round to 00:00:01.000
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "x", Start: 0.0, End: 0.9999},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	vtt := subtitles.FormatVTT(track)
	if !strings.Contains(vtt, "00:00:00.999") {
		t.Errorf("VTT must truncate (not round) to 999ms; got:\n%s", vtt)
	}
	if strings.Contains(vtt, "00:00:01.000") {
		t.Errorf("VTT must not round up to 1000ms; got:\n%s", vtt)
	}
}

func TestFormatSRT_Rounding_TruncatesMilliseconds(t *testing.T) {
	al := providers.Alignment{
		Words: []providers.WordTiming{
			{Word: "y", Start: 0.5005, End: 1.9995},
		},
	}
	track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
	srt := subtitles.FormatSRT(track)
	// 0.5005 * 1000 = 500.5 → truncates to 500
	if !strings.Contains(srt, "00:00:00,500") {
		t.Errorf("SRT start must truncate to 500ms; got:\n%s", srt)
	}
	// 1.9995 * 1000 = 1999.5 → truncates to 1999 → 00:00:01,999
	if !strings.Contains(srt, "00:00:01,999") {
		t.Errorf("SRT end must truncate to 1999ms; got:\n%s", srt)
	}
}

func TestFormatVTT_FractionalSecond_Precision(t *testing.T) {
	// Verify exact millisecond output for several boundary values.
	cases := []struct {
		secs float64
		want string
	}{
		{0.001, "00:00:00.001"},
		{0.999, "00:00:00.999"},
		{1.0, "00:00:01.000"},
		{59.999, "00:00:59.999"},
		{60.0, "00:01:00.000"},
		{3600.0, "01:00:00.000"},
	}
	for _, tc := range cases {
		al := providers.Alignment{
			Words: []providers.WordTiming{{Word: "t", Start: tc.secs, End: tc.secs + 0.5}},
		}
		track := subtitles.BuildTrack(al, subtitles.BuildOptions{})
		vtt := subtitles.FormatVTT(track)
		if !strings.Contains(vtt, tc.want) {
			t.Errorf("%.3fs → want %q in VTT; got:\n%s", tc.secs, tc.want, vtt)
		}
	}
}
