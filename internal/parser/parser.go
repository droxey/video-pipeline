// Package parser parses slides.md and speaker-notes.md source files into a
// structured, validated representation ready for downstream pipeline stages.
//
// Call ParseBoth with the raw bytes of both files. Errors are always collected
// and returned rather than aborting on the first problem, so callers receive the
// full set of issues. A non-empty error slice does not mean the ParseResult is
// nil; partial results are returned to aid diagnosis.
//
// WriteOutput atomically persists the result to normalized.json and any errors
// to parser-errors.json in the supplied directory using the persistence helpers.
package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/nebula/course-video-pipeline/internal/persistence"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	CodeDupSlide          = "E_DUP_SLIDE"
	CodeMissingSlide      = "E_MISSING_SLIDE"
	CodeTitleMismatch     = "E_TITLE_MISMATCH"
	CodeNoApproval        = "E_NO_APPROVAL"
	CodeUnknownMeta       = "E_UNKNOWN_META"
	CodeMalformedHeader   = "E_MALFORMED_HEADER"
	CodeMalformedMeta     = "E_MALFORMED_META"
	CodeMalformedDialogue = "E_MALFORMED_DIALOGUE"
	CodeBadProvenance     = "E_BAD_PROVENANCE"
	CodeBadTimestamp      = "E_BAD_TIMESTAMP"
)

// ─── Public types ─────────────────────────────────────────────────────────────

// ParseError is a structured error with file, line, and slide context.
type ParseError struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Slide   int    `json:"slide,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d [%s] %s", e.File, e.Line, e.Code, e.Message)
	}
	return fmt.Sprintf("%s [%s] %s", e.File, e.Code, e.Message)
}

// Provenance records the origin of a dialogue entry.
type Provenance string

const (
	ProvenanceHuman       Provenance = "human"
	ProvenanceAIGenerated Provenance = "ai-generated"
)

// ApprovalMeta captures who approved a dialogue entry and when.
type ApprovalMeta struct {
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
}

// DialogueEntry is one spoken line, fully attributed and approved.
type DialogueEntry struct {
	Speaker    string       `json:"speaker"`
	Text       string       `json:"text"`
	Provenance Provenance   `json:"provenance"`
	Approval   ApprovalMeta `json:"approval"`
}

// SFXMarker is a sound-effect cue embedded in a slide.
type SFXMarker struct {
	Name string `json:"name"`
}

// ParsedSlide combines slide body/metadata with its approved speaker notes.
type ParsedSlide struct {
	Number          int             `json:"number"`
	Title           string          `json:"title"`
	Body            string          `json:"body"`
	DurationSeconds float64         `json:"duration_seconds,omitempty"`
	SFX             []SFXMarker     `json:"sfx,omitempty"`
	Dialogue        []DialogueEntry `json:"dialogue"`
	ContentHash     string          `json:"content_hash"`
}

// ParseResult is the complete output of a successful parse run.
type ParseResult struct {
	Slides     []ParsedSlide `json:"slides"`
	SlidesHash string        `json:"slides_hash"`
	NotesHash  string        `json:"notes_hash"`
	ParsedAt   time.Time     `json:"parsed_at"`
}

// ─── Shared helpers (used by slides.go and notes.go) ─────────────────────────

// slideHeaderRe matches "## Slide N: Title" at the start of a line.
var slideHeaderRe = regexp.MustCompile(`^##\s+Slide\s+(\d+):\s+(.+)$`)

// metaCommentRe matches an HTML comment of the form "<!-- key: value -->".
var metaCommentRe = regexp.MustCompile(`^<!--\s+(\S+?):\s*(.*?)\s*-->$`)

// section holds the lines belonging to one slide in either source file.
type section struct {
	number     int
	title      string
	headerLine int
	bodyLines  []indexedLine
}

// indexedLine pairs a line of text with its 1-based line number in the source.
type indexedLine struct {
	text string
	num  int
}

// splitLines splits data into lines, normalising CRLF to LF.
func splitLines(data []byte) []string {
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
}

// splitSections divides lines into per-slide sections using ## Slide N: Title
// headers. Lines that precede the first header are silently ignored.
func splitSections(lines []string) []section {
	var sections []section
	var cur *section
	for i, line := range lines {
		lineNum := i + 1
		if m := slideHeaderRe.FindStringSubmatch(line); m != nil {
			n := mustAtoi(m[1])
			title := strings.TrimSpace(m[2])
			if cur != nil {
				sections = append(sections, *cur)
			}
			cur = &section{number: n, title: title, headerLine: lineNum}
		} else if cur != nil {
			cur.bodyLines = append(cur.bodyLines, indexedLine{text: line, num: lineNum})
		}
	}
	if cur != nil {
		sections = append(sections, *cur)
	}
	return sections
}

// sha256hex returns the lowercase hex SHA-256 digest of data.
func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// trimBody strips leading/trailing whitespace from a multi-line string.
func trimBody(lines []string) string {
	return strings.TrimFunc(strings.Join(lines, "\n"), unicode.IsSpace)
}

// mustAtoi converts a decimal string to int; panics only on impossible input
// (caller guarantees the string is already validated by the regex).
func mustAtoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// ─── Entry point ──────────────────────────────────────────────────────────────

// ParseBoth parses the raw bytes of slides.md and speaker-notes.md,
// cross-validates slide numbers/titles, and returns a merged ParseResult plus
// every structural error found. The caller should inspect both return values;
// a non-empty error list does not mean the result is unusable.
func ParseBoth(slidesData, notesData []byte) (*ParseResult, []ParseError) {
	var errs []ParseError

	rawSlides, slideErrs := parseSlides(slidesData)
	errs = append(errs, slideErrs...)

	rawNotes, noteErrs := parseNotes(notesData)
	errs = append(errs, noteErrs...)

	merged, crossErrs := merge(rawSlides, rawNotes)
	errs = append(errs, crossErrs...)

	result := &ParseResult{
		Slides:     merged,
		SlidesHash: sha256hex(slidesData),
		NotesHash:  sha256hex(notesData),
		ParsedAt:   time.Now().UTC(),
	}
	return result, errs
}

// WriteOutput atomically writes normalized.json and parser-errors.json into dir.
// dir must already exist. Both files are written even when errs is non-empty so
// that downstream tooling can always find the error file.
func WriteOutput(dir string, result *ParseResult, errs []ParseError) error {
	if errs == nil {
		errs = []ParseError{} // serialise as [] not null
	}
	if err := persistence.WriteJSON(filepath.Join(dir, "normalized.json"), result); err != nil {
		return err
	}
	return persistence.WriteJSON(filepath.Join(dir, "parser-errors.json"), errs)
}
