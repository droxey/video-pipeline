package render

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

// RemotionRenderer implements FrameRenderer using the Remotion CLI.
//
// # Offline vs. production boundary
//
// For offline/test use, substitute LocalRenderer (write JSON metadata files
// without running any subprocess). Use RemotionRenderer only when a Remotion
// installation is available and a real video output is required.
//
// # Executable discovery
//
// DiscoverRemotionExecutable searches for the Remotion CLI in PATH and common
// Node.js install locations. Use it to populate Executable before first use.
type RemotionRenderer struct {
	// Executable is the path to the Remotion CLI binary (e.g. "remotion" or
	// "/usr/local/lib/node_modules/.bin/remotion").
	Executable string
	// Timeout is the per-slide render deadline. Zero uses DefaultRemotionTimeout.
	Timeout time.Duration
	// ExtraArgs are appended verbatim to every render command, enabling
	// flags like --log-level or --frames-per-lambda for production tuning.
	ExtraArgs []string
}

// DefaultRemotionTimeout is the per-slide render deadline for RemotionRenderer.
const DefaultRemotionTimeout = 5 * time.Minute

// DiscoverRemotionExecutable searches for the Remotion CLI binary. It checks
// PATH first, then common local install paths. Returns an error when the
// executable cannot be found.
func DiscoverRemotionExecutable() (string, error) {
	// 1. System PATH
	if p, err := exec.LookPath("remotion"); err == nil {
		return p, nil
	}
	// 2. Common local Node.js install paths
	candidates := []string{
		"node_modules/.bin/remotion",
		"node_modules/@remotion/cli/bin/remotion.js",
		"/usr/local/lib/node_modules/.bin/remotion",
		"/usr/local/lib/node_modules/@remotion/cli/bin/remotion.js",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("remotion: executable not found in PATH or common locations; install with: npm install -g remotion")
}

// RenderSlide implements FrameRenderer by invoking the Remotion CLI to render
// frames for a single slide into frameDir.
//
// Argument validation is performed before the subprocess is started:
//   - Executable must be non-empty
//   - frameCount must be >= 1
//   - frameDir must be non-empty
//
// The subprocess runs under a deadline derived from Timeout (default:
// DefaultRemotionTimeout). If the context is cancelled the subprocess is killed
// immediately and context.Err() is returned.
func (r *RemotionRenderer) RenderSlide(ctx context.Context, slide domain.Slide, frameStart, frameCount int, frameDir string) error {
	if r.Executable == "" {
		return fmt.Errorf("remotion: Executable is empty; call DiscoverRemotionExecutable or set it explicitly")
	}
	if frameCount < 1 {
		return fmt.Errorf("remotion: frameCount must be >= 1, got %d", frameCount)
	}
	if frameDir == "" {
		return fmt.Errorf("remotion: frameDir must not be empty")
	}
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return fmt.Errorf("remotion: mkdir %s: %w", frameDir, err)
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultRemotionTimeout
	}
	renderCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the frame range: Remotion CLI uses --frames=start-end (inclusive).
	frameEnd := frameStart + frameCount - 1
	args := []string{
		"render",
		"--output", frameDir,
		"--frames", fmt.Sprintf("%d-%d", frameStart, frameEnd),
		"--props", buildRemotionProps(slide),
	}
	args = append(args, r.ExtraArgs...)

	cmd := exec.CommandContext(renderCtx, r.Executable, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if renderCtx.Err() != nil {
			return fmt.Errorf("remotion: slide %d render timed out after %s: %w", slide.Number, timeout, ctx.Err())
		}
		return fmt.Errorf("remotion: slide %d render failed: %w\nstderr: %s",
			slide.Number, err, stderr.String())
	}
	return nil
}

// buildRemotionProps serialises the slide metadata as a JSON string for --props.
func buildRemotionProps(slide domain.Slide) string {
	title := jsonEscape(slide.Title)
	body := jsonEscape(slide.Body)
	dur := strconv.FormatFloat(slide.DurationSeconds, 'f', 3, 64)
	return fmt.Sprintf(`{"number":%d,"title":"%s","body":"%s","duration":%s}`,
		slide.Number, title, body, dur)
}

// jsonEscape escapes a string for safe embedding in a JSON string literal.
func jsonEscape(s string) string {
	var out bytes.Buffer
	for _, r := range s {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// RemotionFrameOutputPath returns the expected frame file path written by
// Remotion for frame index i in frameDir. Remotion zero-pads to 8 digits.
func RemotionFrameOutputPath(frameDir string, frameIndex int) string {
	return filepath.Join(frameDir, fmt.Sprintf("%08d.png", frameIndex))
}
