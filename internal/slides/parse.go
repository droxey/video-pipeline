// Package slides parses slides.md and speaker-notes.md into typed, validated
// structures. All provider and media operations are deliberately absent; this
// package is pure parsing with no paid network calls.
package slides

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ────────── Public types ──────────

// SFXMarker is a sound-effect cue found in content.
// Position is the byte offset in the cleaned (marker-stripped) text where
// the cue should fire.
type SFXMarker struct {
	Preset   string
	Position int
}

// PauseMarker is an explicit pause instruction embedded in narration text.
type PauseMarker struct {
	Seconds  float64
	Position int // byte offset in clean text
}

// ParsedSlide is a fully parsed slide from slides.md.
type ParsedSlide struct {
	Number          int
	Title           string
	DurationSeconds float64
	// Body is the normalized body text with all metadata comments stripped.
	Body           string
	NormalizedHash string // sha256 of Body
	SFX            []SFXMarker
}

// ParsedNote is a fully parsed speaker note from speaker-notes.md.
type ParsedNote struct {
	SlideNumber int
	// SourceIDs lists the recording segment IDs referenced by <!-- source: ... -->.
	// At least one is required for dialogue provenance.
	SourceIDs      []string
	CleanText      string // text with all inline SFX/PAUSE markers stripped
	NormalizedHash string // sha256 of CleanText
	ApprovedBy     string
	ApprovedAt     *time.Time
	SFX            []SFXMarker
	Pauses         []PauseMarker
}

// MatchedSlide pairs a slide with its speaker note.
type MatchedSlide struct {
	Slide *ParsedSlide
	Note  *ParsedNote // nil when no note was found for this slide
}

// ParseError is a non-fatal structural error found during parsing or matching.
type ParseError struct {
	File    string
	Line    int
	Message string
}

func (e ParseError) Error() string {
	switch {
	case e.File != "" && e.Line > 0:
		return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Message)
	case e.File != "":
		return fmt.Sprintf("%s: %s", e.File, e.Message)
	default:
		return e.Message
	}
}

// ParseResult is the output of a full parse-and-match pass.
type ParseResult struct {
	Slides []MatchedSlide
	// CombinedHash is the sha256 of all slide and note hashes concatenated in
	// slide-number order. It is empty when no matched pairs exist.
	CombinedHash string
	Errors       []ParseError
}

// ────────── Compiled regexps ──────────

var (
	// reSlideHead matches "## Slide N" and "## Slide N: Title" (case-insensitive).
	reSlideHead = regexp.MustCompile(`(?i)^##\s+slide\s+(\d+)(?::\s*(.+))?$`)
	// reHTMLComment matches single-line HTML comments <!-- ... -->.
	reHTMLComment = regexp.MustCompile(`<!--(.*?)-->`)
	// reInlineSFX matches [SFX:preset-name] cues.
	reInlineSFX = regexp.MustCompile(`\[SFX:([^\]]+)\]`)
	// reInlinePause matches [PAUSE] and [PAUSE:seconds] cues.
	reInlinePause = regexp.MustCompile(`\[PAUSE(?::([0-9.]+))?\]`)
	// metadata directive patterns (matched against the inner text of HTML comments)
	reMetaDuration   = regexp.MustCompile(`(?i)^\s*duration:\s*([0-9.]+)`)
	reMetaSFX        = regexp.MustCompile(`(?i)^\s*sfx:\s*(.+)`)
	reMetaSource     = regexp.MustCompile(`(?i)^\s*source:\s*(.+)`)
	reMetaApprovedBy = regexp.MustCompile(`(?i)^\s*approved-by:\s*(.+)`)
	reMetaApprovedAt = regexp.MustCompile(`(?i)^\s*approved-at:\s*(.+)`)
)

// ────────── ParseSlides ──────────

// ParseSlides parses the content of a slides.md file. It returns one
// ParsedSlide per heading block and any non-fatal errors encountered.
func ParseSlides(content string) ([]ParsedSlide, []ParseError) {
	var slides []ParsedSlide
	var errs []ParseError

	for _, sec := range splitSections(content) {
		m := reSlideHead.FindStringSubmatch(sec.heading)
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		s := ParsedSlide{Number: num, Title: strings.TrimSpace(m[2])}

		var bodyLines []string
		for _, line := range strings.Split(sec.body, "\n") {
			cm := reHTMLComment.FindStringSubmatch(line)
			if cm == nil {
				bodyLines = append(bodyLines, line)
				continue
			}
			inner := strings.TrimSpace(cm[1])
			switch {
			case reMetaDuration.MatchString(inner):
				dm := reMetaDuration.FindStringSubmatch(inner)
				s.DurationSeconds, _ = strconv.ParseFloat(strings.TrimSpace(dm[1]), 64)
			case reMetaSFX.MatchString(inner):
				sm := reMetaSFX.FindStringSubmatch(inner)
				s.SFX = append(s.SFX, SFXMarker{Preset: strings.TrimSpace(sm[1])})
			}
			// Metadata comments are consumed and not added to body.
		}

		body := normalizeBody(strings.Join(bodyLines, "\n"))
		s.Body = body
		s.NormalizedHash = hashStr(body)
		slides = append(slides, s)
	}

	return slides, errs
}

// ────────── ParseNotes ──────────

// ParseNotes parses the content of a speaker-notes.md file. It returns one
// ParsedNote per heading block and any non-fatal errors encountered.
func ParseNotes(content string) ([]ParsedNote, []ParseError) {
	var notes []ParsedNote
	var errs []ParseError

	for _, sec := range splitSections(content) {
		m := reSlideHead.FindStringSubmatch(sec.heading)
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		n := ParsedNote{SlideNumber: num}

		var textLines []string
		for _, line := range strings.Split(sec.body, "\n") {
			cm := reHTMLComment.FindStringSubmatch(line)
			if cm == nil {
				textLines = append(textLines, line)
				continue
			}
			inner := strings.TrimSpace(cm[1])
			switch {
			case reMetaSource.MatchString(inner):
				sm := reMetaSource.FindStringSubmatch(inner)
				for _, id := range strings.Split(sm[1], ",") {
					if id = strings.TrimSpace(id); id != "" {
						n.SourceIDs = append(n.SourceIDs, id)
					}
				}
			case reMetaApprovedBy.MatchString(inner):
				abm := reMetaApprovedBy.FindStringSubmatch(inner)
				n.ApprovedBy = strings.TrimSpace(abm[1])
			case reMetaApprovedAt.MatchString(inner):
				aam := reMetaApprovedAt.FindStringSubmatch(inner)
				ts := strings.TrimSpace(aam[1])
				t, err := time.Parse(time.RFC3339, ts)
				if err != nil {
					errs = append(errs, ParseError{
						Message: fmt.Sprintf("slide %d: invalid approved-at timestamp %q: %v", num, ts, err),
					})
				} else {
					n.ApprovedAt = &t
				}
			}
			// Metadata comments are consumed and not added to text.
		}

		rawText := normalizeBody(strings.Join(textLines, "\n"))
		clean, sfx, pauses := extractInlineMarkers(rawText)
		n.CleanText = clean
		n.NormalizedHash = hashStr(clean)
		n.SFX = sfx
		n.Pauses = pauses
		notes = append(notes, n)
	}

	return notes, errs
}

// ────────── Match ──────────

// Match pairs slides with notes by slide number and validates dialogue provenance
// and approval. requireApproval mirrors CourseConfig.Approval.RequireFinalConfirmation.
// It never returns a hard error; all structural problems are accumulated in
// ParseResult.Errors so the caller can decide whether to proceed.
func Match(slides []ParsedSlide, notes []ParsedNote, requireApproval bool) ParseResult {
	noteMap := make(map[int]*ParsedNote, len(notes))
	for i := range notes {
		noteMap[notes[i].SlideNumber] = &notes[i]
	}
	slideMap := make(map[int]bool, len(slides))
	for i := range slides {
		slideMap[slides[i].Number] = true
	}

	var result ParseResult
	var hashParts []string

	for i := range slides {
		s := &slides[i]
		n := noteMap[s.Number]
		result.Slides = append(result.Slides, MatchedSlide{Slide: s, Note: n})

		if n == nil {
			result.Errors = append(result.Errors, ParseError{
				Message: fmt.Sprintf("slide %d has no speaker note", s.Number),
			})
			continue
		}
		if len(n.SourceIDs) == 0 {
			result.Errors = append(result.Errors, ParseError{
				Message: fmt.Sprintf("slide %d note missing dialogue provenance (<!-- source: seg-XXXX -->)", s.Number),
			})
		}
		if requireApproval && n.ApprovedBy == "" {
			result.Errors = append(result.Errors, ParseError{
				Message: fmt.Sprintf("slide %d note requires approval but approved-by is absent", s.Number),
			})
		}
		hashParts = append(hashParts, s.NormalizedHash, n.NormalizedHash)
	}

	// Report speaker notes that reference non-existent slides.
	for _, n := range notes {
		if !slideMap[n.SlideNumber] {
			result.Errors = append(result.Errors, ParseError{
				Message: fmt.Sprintf("speaker note for slide %d has no corresponding slide", n.SlideNumber),
			})
		}
	}

	if len(hashParts) > 0 {
		result.CombinedHash = hashStr(strings.Join(hashParts, ":"))
	}
	return result
}

// ────────── Internal helpers ──────────

type section struct {
	heading string
	body    string
}

// splitSections splits markdown content into sections delimited by slide headings.
// Content before the first heading is silently discarded.
func splitSections(content string) []section {
	var sections []section
	var current *section
	var buf strings.Builder

	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		if reSlideHead.MatchString(line) {
			if current != nil {
				current.body = buf.String()
				sections = append(sections, *current)
				buf.Reset()
			}
			current = &section{heading: line}
		} else if current != nil {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if current != nil {
		current.body = buf.String()
		sections = append(sections, *current)
	}
	return sections
}

// extractInlineMarkers strips [SFX:preset] and [PAUSE:N] markers from text,
// recording each marker's byte position in the resulting clean text.
func extractInlineMarkers(text string) (clean string, sfx []SFXMarker, pauses []PauseMarker) {
	type markerLoc struct {
		start, end int
		kind       string
		value      string
	}
	var mlocs []markerLoc

	for _, loc := range reInlineSFX.FindAllStringSubmatchIndex(text, -1) {
		mlocs = append(mlocs, markerLoc{loc[0], loc[1], "sfx", text[loc[2]:loc[3]]})
	}
	for _, loc := range reInlinePause.FindAllStringSubmatchIndex(text, -1) {
		val := ""
		if loc[2] >= 0 {
			val = text[loc[2]:loc[3]]
		}
		mlocs = append(mlocs, markerLoc{loc[0], loc[1], "pause", val})
	}
	sort.Slice(mlocs, func(i, j int) bool { return mlocs[i].start < mlocs[j].start })

	var sb strings.Builder
	prev := 0
	for _, ml := range mlocs {
		sb.WriteString(text[prev:ml.start])
		pos := sb.Len()
		switch ml.kind {
		case "sfx":
			sfx = append(sfx, SFXMarker{Preset: ml.value, Position: pos})
		case "pause":
			secs, _ := strconv.ParseFloat(ml.value, 64)
			if secs <= 0 {
				secs = 0.5 // default pause duration
			}
			pauses = append(pauses, PauseMarker{Seconds: secs, Position: pos})
		}
		prev = ml.end
	}
	sb.WriteString(text[prev:])
	clean = normalizeBody(sb.String())
	return
}

// normalizeBody trims whitespace, preserves paragraph breaks (\n\n), and
// collapses runs of whitespace within each paragraph to a single space.
func normalizeBody(s string) string {
	paras := strings.Split(s, "\n\n")
	out := paras[:0]
	for _, p := range paras {
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

func hashStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
