package quality

import (
	"fmt"
	"strings"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

func ValidateRecording(r *domain.Recording, durationAck bool) []string {
	var errs []string
	if r == nil {
		return []string{"recording is nil"}
	}
	if strings.TrimSpace(r.RecordingID) == "" {
		errs = append(errs, "recording_id is required")
	}
	last := -1.0
	for i, s := range r.Segments {
		if s.End <= s.Start {
			errs = append(errs, fmt.Sprintf("segment %d has non-positive duration", i))
		}
		if s.Start < last {
			errs = append(errs, fmt.Sprintf("segment %d is out of order", i))
		}
		if strings.TrimSpace(s.Text) == "" {
			errs = append(errs, fmt.Sprintf("segment %d has empty text", i))
		}
		last = s.Start
	}
	if r.DurationSeconds <= 0 && !durationAck {
		errs = append(errs, "duration must be positive or explicitly acknowledged")
	}
	narrator := false
	for _, sp := range r.Speakers {
		if sp.Role == domain.SpeakerRoleNarrator {
			narrator = true
		}
	}
	if !narrator {
		errs = append(errs, "narrator speaker is required")
	}
	return errs
}
