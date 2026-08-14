package parser

import (
	"fmt"
	"strconv"
	"strings"
)

const fileSlides = "slides.md"

// allowedSlideMeta is the complete set of recognised metadata keys in slides.md.
var allowedSlideMeta = map[string]bool{
	"duration_seconds": true,
	"sfx":              true,
}

// rawSlide holds the intermediate output of parsing one slide from slides.md.
type rawSlide struct {
	Number          int
	Title           string
	Body            string
	DurationSeconds float64
	SFX             []SFXMarker
	TitleLine       int
}

// parseSlides parses the full contents of slides.md into a slice of rawSlides,
// collecting errors rather than aborting on the first problem.
func parseSlides(data []byte) ([]rawSlide, []ParseError) {
	lines := splitLines(data)
	sections := splitSections(lines)

	var errs []ParseError
	seenAt := make(map[int]int) // slide number → section index (for dup detection)
	var slides []rawSlide

	for _, sec := range sections {
		if prevLine, ok := seenAt[sec.number]; ok {
			errs = append(errs, ParseError{
				File:  fileSlides,
				Line:  sec.headerLine,
				Slide: sec.number,
				Code:  CodeDupSlide,
				Message: fmt.Sprintf("slide %d declared again (first declaration at line %d)",
					sec.number, prevLine),
			})
			continue
		}
		seenAt[sec.number] = sec.headerLine

		slide, secErrs := parseSlideMeta(sec)
		errs = append(errs, secErrs...)
		slides = append(slides, slide)
	}

	return slides, errs
}

// parseSlideMeta extracts duration_seconds, sfx markers, and body text from a
// single slide section. Unknown metadata keys produce E_UNKNOWN_META errors.
func parseSlideMeta(sec section) (rawSlide, []ParseError) {
	var errs []ParseError
	slide := rawSlide{
		Number:    sec.number,
		Title:     sec.title,
		TitleLine: sec.headerLine,
	}

	var bodyLines []string
	for _, il := range sec.bodyLines {
		trimmed := strings.TrimRight(il.text, " \t")
		if m := metaCommentRe.FindStringSubmatch(trimmed); m != nil {
			key := m[1]
			val := strings.TrimSpace(m[2])

			if !allowedSlideMeta[key] {
				errs = append(errs, ParseError{
					File:    fileSlides,
					Line:    il.num,
					Slide:   sec.number,
					Code:    CodeUnknownMeta,
					Message: fmt.Sprintf("unknown metadata key %q in slide %d", key, sec.number),
				})
				continue
			}

			switch key {
			case "duration_seconds":
				v, err := strconv.ParseFloat(val, 64)
				if err != nil || v <= 0 {
					msg := fmt.Sprintf("invalid duration_seconds %q: must be a positive number", val)
					if err != nil {
						msg = fmt.Sprintf("invalid duration_seconds %q: %v", val, err)
					}
					errs = append(errs, ParseError{
						File:    fileSlides,
						Line:    il.num,
						Slide:   sec.number,
						Code:    CodeMalformedMeta,
						Message: msg,
					})
				} else {
					slide.DurationSeconds = v
				}

			case "sfx":
				if val == "" {
					errs = append(errs, ParseError{
						File:    fileSlides,
						Line:    il.num,
						Slide:   sec.number,
						Code:    CodeMalformedMeta,
						Message: "sfx value must not be empty",
					})
				} else {
					slide.SFX = append(slide.SFX, SFXMarker{Name: val})
				}
			}
			continue
		}

		bodyLines = append(bodyLines, il.text)
	}

	slide.Body = trimBody(bodyLines)
	return slide, errs
}
