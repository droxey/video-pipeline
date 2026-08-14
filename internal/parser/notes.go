package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const fileNotes = "speaker-notes.md"

// dialogueRe matches a line of the form "Speaker: dialogue text".
// Speaker names may contain letters, digits, and spaces but must start with a
// letter and must not contain a colon (the colon is the field separator).
var dialogueRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9 ]*?):\s+(.+)$`)

// allowedNotesMeta is the complete set of recognised per-dialogue metadata keys.
var allowedNotesMeta = map[string]bool{
	"provenance":  true,
	"approved-by": true,
	"approved-at": true,
}

// allowedProvenances is the exhaustive set of valid provenance values.
var allowedProvenances = map[Provenance]bool{
	ProvenanceHuman:       true,
	ProvenanceAIGenerated: true,
}

// rawNote is the intermediate output of parsing one slide from speaker-notes.md.
type rawNote struct {
	Number    int
	Title     string
	Dialogue  []rawDialogue
	TitleLine int
}

// rawDialogue is one dialogue entry before it is merged with slide data.
// The has* fields track which approval fields have been set so that we can
// produce precise missing-field errors.
type rawDialogue struct {
	Speaker       string
	Text          string
	Provenance    Provenance
	ApprovedBy    string
	ApprovedAt    time.Time
	Line          int
	hasProvenance bool
	hasApprovedBy bool
	hasApprovedAt bool
}

// parseNotes parses the full contents of speaker-notes.md, collecting errors.
func parseNotes(data []byte) ([]rawNote, []ParseError) {
	lines := splitLines(data)
	sections := splitSections(lines)

	var errs []ParseError
	seenAt := make(map[int]int)
	var notes []rawNote

	for _, sec := range sections {
		if prevLine, ok := seenAt[sec.number]; ok {
			errs = append(errs, ParseError{
				File:  fileNotes,
				Line:  sec.headerLine,
				Slide: sec.number,
				Code:  CodeDupSlide,
				Message: fmt.Sprintf("slide %d declared again (first declaration at line %d)",
					sec.number, prevLine),
			})
			continue
		}
		seenAt[sec.number] = sec.headerLine

		note, secErrs := parseNoteSection(sec)
		errs = append(errs, secErrs...)
		notes = append(notes, note)
	}

	return notes, errs
}

// parseNoteSection parses dialogue entries and their approval metadata from one
// speaker-notes.md slide section. Each dialogue line must be followed by
// provenance, approved-by, and approved-at metadata before the next dialogue
// line or end of section; missing fields produce E_NO_APPROVAL errors.
func parseNoteSection(sec section) (rawNote, []ParseError) {
	var errs []ParseError
	note := rawNote{
		Number:    sec.number,
		Title:     sec.title,
		TitleLine: sec.headerLine,
	}

	var pending *rawDialogue

	flush := func() {
		if pending == nil {
			return
		}
		var missing []string
		if !pending.hasProvenance {
			missing = append(missing, "provenance")
		}
		if !pending.hasApprovedBy {
			missing = append(missing, "approved-by")
		}
		if !pending.hasApprovedAt {
			missing = append(missing, "approved-at")
		}
		if len(missing) > 0 {
			errs = append(errs, ParseError{
				File:  fileNotes,
				Line:  pending.Line,
				Slide: sec.number,
				Code:  CodeNoApproval,
				Message: fmt.Sprintf("dialogue on line %d is missing required approval field(s): %s",
					pending.Line, strings.Join(missing, ", ")),
			})
		}
		note.Dialogue = append(note.Dialogue, *pending)
		pending = nil
	}

	for _, il := range sec.bodyLines {
		trimmed := strings.TrimSpace(il.text)

		// Skip blank lines and visual separators; they do not delimit entries.
		if trimmed == "" || trimmed == "---" {
			continue
		}

		// ── Metadata comment ─────────────────────────────────────────────────
		if m := metaCommentRe.FindStringSubmatch(trimmed); m != nil {
			key := m[1]
			val := strings.TrimSpace(m[2])

			if !allowedNotesMeta[key] {
				errs = append(errs, ParseError{
					File:    fileNotes,
					Line:    il.num,
					Slide:   sec.number,
					Code:    CodeUnknownMeta,
					Message: fmt.Sprintf("unknown metadata key %q", key),
				})
				continue
			}

			if pending == nil {
				errs = append(errs, ParseError{
					File:    fileNotes,
					Line:    il.num,
					Slide:   sec.number,
					Code:    CodeMalformedMeta,
					Message: fmt.Sprintf("metadata %q appears before any dialogue line in slide %d", key, sec.number),
				})
				continue
			}

			switch key {
			case "provenance":
				prov := Provenance(val)
				if !allowedProvenances[prov] {
					errs = append(errs, ParseError{
						File:    fileNotes,
						Line:    il.num,
						Slide:   sec.number,
						Code:    CodeBadProvenance,
						Message: fmt.Sprintf("unknown provenance %q (valid values: human, ai-generated)", val),
					})
				} else {
					pending.Provenance = prov
					pending.hasProvenance = true
				}

			case "approved-by":
				if val == "" {
					errs = append(errs, ParseError{
						File:    fileNotes,
						Line:    il.num,
						Slide:   sec.number,
						Code:    CodeMalformedMeta,
						Message: "approved-by value must not be empty",
					})
				} else {
					pending.ApprovedBy = val
					pending.hasApprovedBy = true
				}

			case "approved-at":
				t, err := time.Parse(time.RFC3339, val)
				if err != nil {
					errs = append(errs, ParseError{
						File:    fileNotes,
						Line:    il.num,
						Slide:   sec.number,
						Code:    CodeBadTimestamp,
						Message: fmt.Sprintf("invalid approved-at timestamp %q: %v", val, err),
					})
				} else {
					pending.ApprovedAt = t.UTC()
					pending.hasApprovedAt = true
				}
			}
			continue
		}

		// ── Dialogue line ─────────────────────────────────────────────────────
		if m := dialogueRe.FindStringSubmatch(trimmed); m != nil {
			flush()
			pending = &rawDialogue{
				Speaker: strings.TrimSpace(m[1]),
				Text:    strings.TrimSpace(m[2]),
				Line:    il.num,
			}
			continue
		}

		// ── Unrecognised line ─────────────────────────────────────────────────
		errs = append(errs, ParseError{
			File:    fileNotes,
			Line:    il.num,
			Slide:   sec.number,
			Code:    CodeMalformedDialogue,
			Message: fmt.Sprintf("unrecognised line in slide %d notes (expected 'Speaker: text' or metadata comment)", sec.number),
		})
	}

	flush()
	return note, errs
}
