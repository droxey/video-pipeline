package grain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

const sourceGrain = "grain"

// ToRecording converts Grain utterances and metadata into a domain.Recording.
func ToRecording(meta Meta, utterances []Utterance, opts Options) (*domain.Recording, error) {
	if strings.TrimSpace(meta.RecordingID) == "" {
		return nil, fmt.Errorf("grain: recording_id is required")
	}
	if len(utterances) == 0 {
		return nil, fmt.Errorf("grain: no utterances to convert")
	}

	sorted := append([]Utterance(nil), utterances...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start == sorted[j].Start {
			return sorted[i].End < sorted[j].End
		}
		return sorted[i].Start < sorted[j].Start
	})

	speakers := buildSpeakers(sorted)
	assignRoles(speakers, opts.Narrator)

	segments, err := buildSegments(sorted)
	if err != nil {
		return nil, err
	}

	duration := computeDuration(sorted, meta.DurationSeconds)

	return &domain.Recording{
		RecordingID:     meta.RecordingID,
		Source:          sourceGrain,
		SourceURL:       meta.SourceURL,
		Title:           meta.Title,
		RecordedAt:      meta.RecordedAt,
		DurationSeconds: duration,
		Speakers:        speakers,
		Segments:        segments,
	}, nil
}

type speakerInfo struct {
	sourceID    string
	displayName string
}

func buildSpeakers(utterances []Utterance) []domain.Speaker {
	byID := make(map[string]*speakerInfo)
	order := make([]string, 0)

	for _, u := range utterances {
		id := strings.TrimSpace(u.ParticipantID)
		if id == "" {
			return nil
		}
		name := strings.TrimSpace(u.Speaker)
		if name == "" {
			name = id
		}
		if existing, ok := byID[id]; ok {
			if existing.displayName == "" && name != "" {
				existing.displayName = name
			}
			continue
		}
		byID[id] = &speakerInfo{
			sourceID:    id,
			displayName: name,
		}
		order = append(order, id)
	}

	speakers := make([]domain.Speaker, 0, len(order))
	for _, id := range order {
		info := byID[id]
		speakers = append(speakers, domain.Speaker{
			SourceID:    info.sourceID,
			DisplayName: info.displayName,
			Role:        domain.SpeakerRoleParticipant,
		})
	}
	return speakers
}

// assignRoles picks the narrator using, in order:
//  1. A speaker whose display name matches opts.Narrator (case-insensitive).
//  2. The sole speaker when there is exactly one.
//  3. The first speaker in utterance order; all others remain participants.
func assignRoles(speakers []domain.Speaker, narratorName string) {
	if len(speakers) == 0 {
		return
	}

	narratorIdx := -1
	if trimmed := strings.TrimSpace(narratorName); trimmed != "" {
		want := strings.ToLower(trimmed)
		for i := range speakers {
			if strings.ToLower(speakers[i].DisplayName) == want {
				narratorIdx = i
				break
			}
		}
	}

	if narratorIdx < 0 && len(speakers) == 1 {
		narratorIdx = 0
	}
	if narratorIdx < 0 {
		narratorIdx = 0
	}

	speakers[narratorIdx].Role = domain.SpeakerRoleNarrator
}

func buildSegments(utterances []Utterance) ([]domain.Segment, error) {
	segments := make([]domain.Segment, 0, len(utterances))
	for i, u := range utterances {
		text := strings.TrimSpace(u.Text)
		if text == "" {
			return nil, fmt.Errorf("grain: utterance %d has empty text", i)
		}
		id := strings.TrimSpace(u.ParticipantID)
		if id == "" {
			return nil, fmt.Errorf("grain: utterance %d missing participant_id", i)
		}
		start := msToSeconds(u.Start)
		end := msToSeconds(u.End)
		if end <= start {
			return nil, fmt.Errorf("grain: utterance %d has non-positive duration", i)
		}
		segments = append(segments, domain.Segment{
			ID:        fmt.Sprintf("seg-%04d", i+1),
			SpeakerID: id,
			Start:     start,
			End:       end,
			Text:      text,
		})
	}
	return segments, nil
}

func computeDuration(utterances []Utterance, override *float64) float64 {
	if override != nil && *override > 0 {
		return *override
	}
	last := utterances[len(utterances)-1]
	return msToSeconds(last.End)
}

func msToSeconds(ms int64) float64 {
	return float64(ms) / 1000.0
}
