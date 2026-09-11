package grain

import (
	"encoding/json"
	"fmt"
)

// ParseTranscriptJSON decodes Grain transcript.json bytes into utterances.
// The file is a JSON array of utterance objects with millisecond timestamps.
func ParseTranscriptJSON(data []byte) ([]Utterance, error) {
	var utterances []Utterance
	if err := json.Unmarshal(data, &utterances); err != nil {
		return nil, fmt.Errorf("grain: parse transcript.json: %w", err)
	}
	if len(utterances) == 0 {
		return nil, fmt.Errorf("grain: transcript.json is empty")
	}
	return utterances, nil
}
