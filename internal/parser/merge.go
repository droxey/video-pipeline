package parser

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// merge cross-validates rawSlides and rawNotes: every slide number must appear
// in both files with identical titles. For matched pairs it builds a ParsedSlide
// with a deterministic content hash. Structural mismatches produce typed errors.
func merge(slides []rawSlide, notes []rawNote) ([]ParsedSlide, []ParseError) {
	var errs []ParseError

	slideByNum := make(map[int]rawSlide, len(slides))
	for _, s := range slides {
		slideByNum[s.Number] = s
	}
	noteByNum := make(map[int]rawNote, len(notes))
	for _, n := range notes {
		noteByNum[n.Number] = n
	}

	// Collect the union of all slide numbers and iterate in order.
	allNums := make(map[int]bool, len(slides)+len(notes))
	for _, s := range slides {
		allNums[s.Number] = true
	}
	for _, n := range notes {
		allNums[n.Number] = true
	}
	nums := make([]int, 0, len(allNums))
	for num := range allNums {
		nums = append(nums, num)
	}
	sort.Ints(nums)

	var result []ParsedSlide
	for _, num := range nums {
		s, hasSlide := slideByNum[num]
		n, hasNote := noteByNum[num]

		if !hasSlide {
			errs = append(errs, ParseError{
				File:    fileNotes,
				Line:    n.TitleLine,
				Slide:   num,
				Code:    CodeMissingSlide,
				Message: fmt.Sprintf("slide %d found in speaker-notes.md but missing from slides.md", num),
			})
			continue
		}
		if !hasNote {
			errs = append(errs, ParseError{
				File:    fileSlides,
				Line:    s.TitleLine,
				Slide:   num,
				Code:    CodeMissingSlide,
				Message: fmt.Sprintf("slide %d found in slides.md but missing from speaker-notes.md", num),
			})
			continue
		}

		if s.Title != n.Title {
			errs = append(errs, ParseError{
				File:  fileNotes,
				Line:  n.TitleLine,
				Slide: num,
				Code:  CodeTitleMismatch,
				Message: fmt.Sprintf("slide %d title mismatch: slides.md has %q, speaker-notes.md has %q",
					num, s.Title, n.Title),
			})
			// slides.md is authoritative; continue building the slide anyway.
		}

		dialogue := make([]DialogueEntry, 0, len(n.Dialogue))
		for _, d := range n.Dialogue {
			dialogue = append(dialogue, DialogueEntry{
				Speaker:    d.Speaker,
				Text:       d.Text,
				Provenance: d.Provenance,
				Approval: ApprovalMeta{
					ApprovedBy: d.ApprovedBy,
					ApprovedAt: d.ApprovedAt,
				},
			})
		}

		ps := ParsedSlide{
			Number:          s.Number,
			Title:           s.Title,
			Body:            s.Body,
			DurationSeconds: s.DurationSeconds,
			SFX:             s.SFX,
			Dialogue:        dialogue,
		}
		ps.ContentHash = slideContentHash(ps)
		result = append(result, ps)
	}

	return result, errs
}

// slideContentHash returns a deterministic lowercase hex SHA-256 digest derived
// from all fields that constitute the slide's identity. The hash changes when
// any text, speaker, provenance, or approval attribution changes.
func slideContentHash(ps ParsedSlide) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "number:%d\ntitle:%s\nbody:%s\nduration:%g\n",
		ps.Number, ps.Title, ps.Body, ps.DurationSeconds)
	for _, sfx := range ps.SFX {
		fmt.Fprintf(&sb, "sfx:%s\n", sfx.Name)
	}
	for _, d := range ps.Dialogue {
		fmt.Fprintf(&sb, "dialogue:%s:%s:%s:%s:%s\n",
			d.Speaker, d.Text, d.Provenance,
			d.Approval.ApprovedBy,
			d.Approval.ApprovedAt.UTC().Format(time.RFC3339))
	}
	return sha256hex([]byte(sb.String()))
}
