package render

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

// LocalRenderer is a deterministic, executable-free FrameRenderer for tests
// and offline pipelines. For each slide it writes one JSON metadata file per
// frame into frameDir instead of generating real video frames.
type LocalRenderer struct {
	// ForceErr, when non-nil, is returned from every RenderSlide call.
	ForceErr error
	// CallCount is incremented on each RenderSlide call.
	CallCount int
}

// RenderSlide implements FrameRenderer without any subprocess.
// It writes one JSON file per frame named <frameStart+i>.json into frameDir.
func (r *LocalRenderer) RenderSlide(_ context.Context, slide domain.Slide, frameStart, frameCount int, frameDir string) error {
	r.CallCount++
	if r.ForceErr != nil {
		return r.ForceErr
	}
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return fmt.Errorf("local renderer: mkdir: %w", err)
	}
	for i := 0; i < frameCount; i++ {
		name := filepath.Join(frameDir, fmt.Sprintf("%08d.json", frameStart+i))
		line := fmt.Sprintf(`{"frame":%d,"slide":%d,"title":%q}`+"\n",
			frameStart+i, slide.Number, slide.Title)
		if err := os.WriteFile(name, []byte(line), 0o644); err != nil {
			return fmt.Errorf("local renderer: write frame %d: %w", frameStart+i, err)
		}
	}
	return nil
}

// LocalMixer is a deterministic, executable-free AudioMixer for tests and
// offline pipelines. It writes a text file listing all input paths into
// outputPath instead of producing real mixed audio.
type LocalMixer struct {
	// ForceErr, when non-nil, is returned from every MixAudio call.
	ForceErr error
	// CallCount is incremented on each MixAudio call.
	CallCount int
}

// MixAudio implements AudioMixer without any subprocess.
func (m *LocalMixer) MixAudio(_ context.Context, inputPaths []string, outputPath string) error {
	m.CallCount++
	if m.ForceErr != nil {
		return m.ForceErr
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("local mixer: mkdir: %w", err)
	}
	var content string
	for _, p := range inputPaths {
		content += p + "\n"
	}
	return os.WriteFile(outputPath, []byte(content), 0o644)
}
