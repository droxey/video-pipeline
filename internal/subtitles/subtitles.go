// Package subtitles builds WebVTT and SRT subtitle tracks from forced-alignment
// word timings. It is pure text-processing with no external dependencies.
package subtitles

import (
	"fmt"
	"strings"

	"github.com/nebula/course-video-pipeline/internal/domain"
	"github.com/nebula/course-video-pipeline/internal/providers"
)

// DefaultWordsPerCue is used when BuildTrack is called without explicit options.
const DefaultWordsPerCue = 7

// BuildOptions controls how words are grouped into subtitle cues.
type BuildOptions struct {
	// WordsPerCue is the maximum number of words per subtitle cue.
	// Zero uses DefaultWordsPerCue.
	WordsPerCue int
	// Language is the BCP-47 tag written into the SubtitleTrack.
	// Defaults to "en" when empty.
	Language string
	// Format is "vtt" or "srt". Defaults to "vtt" when empty.
	Format string
}

// BuildTrack groups word timings from al into subtitle cues and returns a
// SubtitleTrack ready for FormatVTT or FormatSRT.
func BuildTrack(al providers.Alignment, opts BuildOptions) domain.SubtitleTrack {
	wpc := opts.WordsPerCue
	if wpc <= 0 {
		wpc = DefaultWordsPerCue
	}
	lang := opts.Language
	if lang == "" {
		lang = "en"
	}
	format := opts.Format
	if format == "" {
		format = "vtt"
	}

	var cues []domain.SubtitleCue
	words := al.Words
	for i := 0; i < len(words); i += wpc {
		end := i + wpc
		if end > len(words) {
			end = len(words)
		}
		group := words[i:end]
		texts := make([]string, len(group))
		for j, w := range group {
			texts[j] = w.Word
		}
		cues = append(cues, domain.SubtitleCue{
			Index: len(cues) + 1,
			Start: group[0].Start,
			End:   group[len(group)-1].End,
			Text:  strings.Join(texts, " "),
		})
	}

	return domain.SubtitleTrack{Language: lang, Format: format, Cues: cues}
}

// FormatVTT renders track as a WebVTT string.
func FormatVTT(track domain.SubtitleTrack) string {
	var sb strings.Builder
	sb.WriteString("WEBVTT\n\n")
	for _, c := range track.Cues {
		fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n",
			c.Index,
			vttTime(c.Start),
			vttTime(c.End),
			c.Text,
		)
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// FormatSRT renders track as an SRT string.
func FormatSRT(track domain.SubtitleTrack) string {
	var sb strings.Builder
	for _, c := range track.Cues {
		fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n",
			c.Index,
			srtTime(c.Start),
			srtTime(c.End),
			c.Text,
		)
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// vttTime formats seconds as HH:MM:SS.mmm for WebVTT.
func vttTime(seconds float64) string {
	ms := int(seconds*1000) % 1000
	total := int(seconds)
	ss := total % 60
	total /= 60
	mm := total % 60
	hh := total / 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hh, mm, ss, ms)
}

// srtTime formats seconds as HH:MM:SS,mmm for SRT.
func srtTime(seconds float64) string {
	ms := int(seconds*1000) % 1000
	total := int(seconds)
	ss := total % 60
	total /= 60
	mm := total % 60
	hh := total / 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hh, mm, ss, ms)
}
