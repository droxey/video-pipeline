package grain

import "time"

// Utterance is one entry from a Grain transcript.json export.
type Utterance struct {
	Start         int64  `json:"start"`
	End           int64  `json:"end"`
	Text          string `json:"text"`
	ParticipantID string `json:"participant_id"`
	Speaker       string `json:"speaker"`
}

// Meta carries recording-level metadata not present in transcript.json.
type Meta struct {
	RecordingID     string
	Title           string
	SourceURL       string
	RecordedAt      time.Time
	DurationSeconds *float64 // optional override; computed from utterances when nil
}

// Options configures transcript conversion behavior.
type Options struct {
	// Narrator is an optional display name to assign the narrator role.
	// When empty, a single speaker becomes narrator; with multiple speakers
	// the first speaker in utterance order is narrator and the rest are participants.
	Narrator string
}
