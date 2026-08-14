package providers

import (
	"context"
	"github.com/nebula/course-video-pipeline/internal/domain"
	"io"
)

type Grain interface {
	Shortlist(context.Context, TimeRange) ([]RecordingCandidate, error)
	Import(context.Context, string) (*domain.Recording, error)
}
type TimeRange struct{ Since, Until string }
type RecordingCandidate struct {
	ID, Title, URL  string
	DurationSeconds float64
}
type VoiceVerifier interface {
	VerifyVoice(context.Context, string) (bool, error)
}
type TTS interface {
	Synthesize(context.Context, string, string, io.Writer) (Usage, error)
}
type Aligner interface {
	Align(context.Context, io.Reader, string) (Alignment, error)
}
type Renderer interface {
	RenderSilent(context.Context, []domain.Slide, string) error
}
type Mixer interface {
	Mix(context.Context, string, []string, string) error
}
type Usage struct {
	Provider, Operation string
	Characters          int
}
type WordTiming struct {
	Word       string
	Start, End float64
}
type Alignment struct {
	Words []WordTiming
	Loss  float64
}
