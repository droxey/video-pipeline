package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nebula/course-video-pipeline/internal/grain"
	"github.com/nebula/course-video-pipeline/internal/quality"
)

func runGrain(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pipeline grain import <transcript.json> [flags]")
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		runGrainImport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown grain subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: pipeline grain import <transcript.json> [flags]")
		os.Exit(1)
	}
}

func runGrainImport(args []string) {
	fs := flag.NewFlagSet("grain import", flag.ExitOnError)
	id := fs.String("id", "", "recording ID (required)")
	title := fs.String("title", "", "recording title")
	sourceURL := fs.String("url", "", "Grain share URL")
	recordedAt := fs.String("recorded-at", "", "ISO8601 recording timestamp (default: now UTC)")
	narrator := fs.String("narrator", "", "display name of the narrator speaker")
	duration := fs.Float64("duration", 0, "override duration in seconds (default: from transcript)")
	out := fs.String("o", "", "write Recording JSON to file (default: stdout)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pipeline grain import <transcript.json> [flags]")
		fs.PrintDefaults()
	}
	if len(args) < 1 {
		fs.Usage()
		os.Exit(1)
	}
	transcriptPath := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		os.Exit(1)
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "error: --id is required")
		os.Exit(1)
	}

	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading transcript: %v\n", err)
		os.Exit(1)
	}

	utterances, err := grain.ParseTranscriptJSON(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing transcript: %v\n", err)
		os.Exit(1)
	}

	ts := time.Now().UTC()
	if *recordedAt != "" {
		ts, err = time.Parse(time.RFC3339, *recordedAt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing --recorded-at: %v\n", err)
			os.Exit(1)
		}
	}

	meta := grain.Meta{
		RecordingID: *id,
		Title:       *title,
		SourceURL:   *sourceURL,
		RecordedAt:  ts,
	}
	if *duration > 0 {
		meta.DurationSeconds = duration
	}

	rec, err := grain.ToRecording(meta, utterances, grain.Options{Narrator: *narrator})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error converting transcript: %v\n", err)
		os.Exit(1)
	}

	if errs := quality.ValidateRecording(rec, meta.DurationSeconds != nil); len(errs) != 0 {
		fmt.Fprintf(os.Stderr, "warning: recording validation issues:\n")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
	}

	encoded, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding JSON: %v\n", err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')

	if *out == "" {
		os.Stdout.Write(encoded)
		return
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}
}
