// Command pipeline is the Nebula course video pipeline CLI.
// This binary implements the infrastructure vertical slice (v1.1-infra).
// Paid provider integrations (ElevenLabs, music, SFX) are not yet wired.
package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.1.0-infra"

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "version":
		fmt.Printf("nebula-pipeline %s\n", version)
	case "grain":
		fmt.Fprintln(os.Stderr, "grain: not yet implemented (infrastructure slice)")
		os.Exit(1)
	case "parse":
		fmt.Fprintln(os.Stderr, "parse: not yet implemented (infrastructure slice)")
		os.Exit(1)
	case "review":
		fmt.Fprintln(os.Stderr, "review: not yet implemented (infrastructure slice)")
		os.Exit(1)
	case "render":
		fmt.Fprintln(os.Stderr, "render: not yet implemented (infrastructure slice)")
		os.Exit(1)
	case "run":
		fmt.Fprintln(os.Stderr, "run: not yet implemented (infrastructure slice)")
		os.Exit(1)
	case "status":
		fmt.Fprintln(os.Stderr, "status: not yet implemented (infrastructure slice)")
		os.Exit(1)
	case "usage":
		fmt.Fprintln(os.Stderr, "usage: not yet implemented (infrastructure slice)")
		os.Exit(1)
	case "cleanup":
		fmt.Fprintln(os.Stderr, "cleanup: not yet implemented (infrastructure slice)")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", flag.Arg(0))
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: pipeline <command> [flags]

Commands:
  version     Print version
  grain       Grain recording operations (shortlist, import)
  parse       Parse normalized course assets
  review      Manage the human review queue
  render      Render pipeline stages (silent, audio, final)
  run         Execute the full pipeline
  status      Show run status
  usage       Show usage report
  cleanup     Remove temporary and failed artifacts

`)
}
