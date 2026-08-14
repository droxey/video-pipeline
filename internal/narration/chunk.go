// Package narration splits cleaned narration text into TTS-ready chunks while
// respecting protected tokens, sentence/clause boundaries, character limits,
// dialogue provenance, and continuity ordering. No paid network calls are made.
package narration

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ────────── Public types ──────────

// Input is one slide's cleaned narration text (markers already stripped by the
// slides package) ready for chunking.
type Input struct {
	SlideNumber int
	// Text is the clean narration text. Paragraph breaks (\n\n) are treated as
	// mandatory split points regardless of accumulated character count.
	Text      string
	SourceIDs []string // recording segment IDs for dialogue provenance
}

// Chunk is a single TTS-ready narration segment.
type Chunk struct {
	// ID is a deterministic 16-hex-char string derived from SlideNumber and
	// OrderIndex. It is stable as long as the slide content and ordering do not change.
	ID          string
	Text        string
	SlideNumber int
	// SourceIDs is copied from the originating Input for downstream provenance.
	SourceIDs []string
	// Seed is a deterministic int64 for reproducible downstream operations such as
	// TTS cache keying or music continuity scheduling. It is derived from the
	// chunk text hash XOR'd with GlobalSeed.
	Seed       int64
	OrderIndex int // 0-based global position in the full narration sequence
	CharCount  int
}

// ChunkError records a chunk that was rejected because it could not be split
// further yet still exceeded MaxChars (e.g., a protected token longer than the limit).
type ChunkError struct {
	SlideNumber int
	OrderIndex  int
	Text        string
	Err         string
}

func (e ChunkError) Error() string {
	return fmt.Sprintf("slide %d chunk %d: %s", e.SlideNumber, e.OrderIndex, e.Err)
}

// Options controls how inputs are split into chunks.
type Options struct {
	// MaxChars is the hard character limit per chunk. 0 means no limit.
	MaxChars int
	// ProtectedPatterns are compiled to regexp.Regexp values. Matches must not
	// be split across chunk boundaries (e.g., URLs, inline code, version strings).
	ProtectedPatterns []string
	// GlobalSeed is XOR'd into each chunk's Seed for domain-level replay isolation.
	// The zero value is valid and produces a purely text-derived seed.
	GlobalSeed int64
}

// DefaultOptions returns sensible defaults aligned with typical TTS API limits.
func DefaultOptions() Options {
	return Options{
		MaxChars: 500,
		ProtectedPatterns: []string{
			"`[^`]+`",                            // inline code
			`https?://\S+`,                       // URLs
			`\b[A-Z][A-Z0-9]*(?:/[A-Z0-9.]+)+\b`, // e.g. HTTP/2, TLS/1.3
			`\b\d+(?:\.\d+)+\b`,                  // version numbers e.g. 1.22.0
		},
	}
}

// Result holds all accepted chunks and any rejection errors from a Split call.
type Result struct {
	Chunks []Chunk
	Errors []ChunkError
}

// ScheduledChunk wraps a Chunk with predecessor/successor IDs for continuity
// scheduling. The linked list forms a total order over the full narration.
type ScheduledChunk struct {
	Chunk
	// PredecessorID is the ID of the previous chunk. Empty for the first chunk.
	PredecessorID string
	// SuccessorID is the ID of the next chunk. Empty for the last chunk.
	SuccessorID string
}

// ContinuitySchedule is an ordered sequence of ScheduledChunks with linked
// predecessor/successor IDs. It is safe to range over directly for synthesis.
type ContinuitySchedule []ScheduledChunk

// ────────── Split ──────────

// Split splits each Input's text into TTS-ready Chunks, accumulating sentence
// and clause fragments up to MaxChars. Paragraph breaks in the text (\n\n) are
// treated as mandatory split points. Chunks are emitted in source order.
//
// A fragment that cannot be split further (because it contains only a single
// protected token or boundary atom) but still exceeds MaxChars is recorded as
// a ChunkError and excluded from Result.Chunks.
func Split(inputs []Input, opts Options) Result {
	compiled := compileProtected(opts.ProtectedPatterns)
	var result Result
	globalIndex := 0

	for _, inp := range inputs {
		if strings.TrimSpace(inp.Text) == "" {
			continue
		}

		// Paragraph breaks are hard boundaries; split there first.
		paragraphs := strings.Split(inp.Text, "\n\n")
		for _, para := range paragraphs {
			para = strings.TrimSpace(para)
			if para == "" {
				continue
			}
			atoms := splitAtoms(para, compiled)
			var cur strings.Builder

			flush := func() {
				text := strings.TrimSpace(cur.String())
				cur.Reset()
				if text == "" {
					return
				}
				if opts.MaxChars > 0 && len(text) > opts.MaxChars {
					result.Errors = append(result.Errors, ChunkError{
						SlideNumber: inp.SlideNumber,
						OrderIndex:  globalIndex,
						Text:        text,
						Err: fmt.Sprintf("chunk exceeds MaxChars %d (len=%d); cannot split further without breaking protected tokens or sentence boundaries",
							opts.MaxChars, len(text)),
					})
					globalIndex++
					return
				}
				result.Chunks = append(result.Chunks, makeChunk(inp, text, globalIndex, opts.GlobalSeed))
				globalIndex++
			}

			for _, atom := range atoms {
				sep := " "
				if cur.Len() == 0 {
					sep = ""
				}
				proposed := cur.Len() + len(sep) + len(atom)
				if opts.MaxChars > 0 && cur.Len() > 0 && proposed > opts.MaxChars {
					flush()
				}
				if cur.Len() > 0 {
					cur.WriteByte(' ')
				}
				cur.WriteString(atom)
			}
			flush()
		}
	}

	return result
}

// ────────── Schedule ──────────

// Schedule builds a ContinuitySchedule from a Result. Chunks are ordered by
// OrderIndex and linked via PredecessorID/SuccessorID to form a total order
// suitable for sequential TTS synthesis.
func Schedule(r Result) ContinuitySchedule {
	chunks := make([]Chunk, len(r.Chunks))
	copy(chunks, r.Chunks)
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].OrderIndex < chunks[j].OrderIndex
	})

	sched := make(ContinuitySchedule, len(chunks))
	for i, c := range chunks {
		sc := ScheduledChunk{Chunk: c}
		if i > 0 {
			sc.PredecessorID = chunks[i-1].ID
		}
		if i < len(chunks)-1 {
			sc.SuccessorID = chunks[i+1].ID
		}
		sched[i] = sc
	}
	return sched
}

// ────────── Internal helpers ──────────

func compileProtected(patterns []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

// splitAtoms splits text at sentence-end (., !, ?) and clause (, ; :) boundaries,
// skipping boundary characters that fall inside a protected token region.
// Each returned atom is trimmed of surrounding whitespace.
func splitAtoms(text string, protected []*regexp.Regexp) []string {
	type span struct{ lo, hi int }
	var spans []span
	for _, re := range protected {
		for _, loc := range re.FindAllStringIndex(text, -1) {
			spans = append(spans, span{loc[0], loc[1]})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })

	inProtected := func(pos int) bool {
		for _, s := range spans {
			if pos >= s.lo && pos < s.hi {
				return true
			}
		}
		return false
	}

	var atoms []string
	start := 0
	n := len(text)

	for i := 0; i < n; i++ {
		if inProtected(i) {
			continue
		}
		c := text[i]
		sentEnd := (c == '.' || c == '!' || c == '?') &&
			(i+1 >= n || text[i+1] == ' ' || text[i+1] == '\n')
		clauseEnd := (c == ',' || c == ';' || c == ':') &&
			i+1 < n && text[i+1] == ' '

		if sentEnd || clauseEnd {
			atom := strings.TrimSpace(text[start : i+1])
			if atom != "" {
				atoms = append(atoms, atom)
			}
			start = i + 1
			if start < n && (text[start] == ' ' || text[start] == '\n') {
				start++
			}
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		atoms = append(atoms, tail)
	}
	return atoms
}

func makeChunk(inp Input, text string, orderIndex int, globalSeed int64) Chunk {
	return Chunk{
		ID:          chunkID(inp.SlideNumber, orderIndex),
		Text:        text,
		SlideNumber: inp.SlideNumber,
		SourceIDs:   inp.SourceIDs,
		Seed:        textSeed(text) ^ globalSeed,
		OrderIndex:  orderIndex,
		CharCount:   len(text),
	}
}

// chunkID produces a deterministic 16-hex-char ID from slide number and order index.
func chunkID(slideNum, orderIndex int) string {
	raw := fmt.Sprintf("s%d:i%d", slideNum, orderIndex)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:8])
}

// textSeed derives a deterministic int64 seed from the SHA-256 of text.
func textSeed(text string) int64 {
	sum := sha256.Sum256([]byte(text))
	return int64(binary.BigEndian.Uint64(sum[:8]))
}
