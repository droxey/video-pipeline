# Video Pipeline

Nebula course video pipeline — converts Grain recordings and course assets into rendered video.

## Grain import

Convert a Grain `00_GRAIN` export `transcript.json` into normalized `domain.Recording` JSON for the import stage.

Grain exports utterances as a JSON array with millisecond timestamps:

```json
[
  {"start": 330, "end": 4200, "text": "...", "participant_id": "...", "speaker": "Display Name"}
]
```

### CLI

```bash
go run ./cmd/pipeline grain import \
  path/to/00_GRAIN/<recording-id>/transcript.json \
  --id grain-rec-00abc123 \
  --title "Event-Driven Architecture Deep Dive" \
  --url "https://grain.com/share/grain-rec-00abc123" \
  --recorded-at 2026-08-02T12:00:00Z \
  --narrator "Dani Roxberry" \
  -o recording.json
```

Flags:

| Flag | Description |
|------|-------------|
| `--id` | Recording ID (required) |
| `--title` | Human-readable title |
| `--url` | Grain share URL |
| `--recorded-at` | ISO8601 timestamp (default: now UTC) |
| `--narrator` | Display name to assign the narrator role |
| `--duration` | Override duration in seconds |
| `-o` | Output file (default: stdout) |

### Narrator assignment

`quality.ValidateRecording` requires exactly one speaker with role `narrator`. The adapter picks the narrator using:

1. **Explicit flag** — `--narrator "Dani Roxberry"` matches a speaker display name (case-insensitive).
2. **Single speaker** — the only speaker becomes narrator.
3. **Fallback** — the first speaker in utterance order is narrator; others are `participant`.

### Library

```go
utterances, _ := grain.ParseTranscriptJSON(data)
rec, _ := grain.ToRecording(grain.Meta{
    RecordingID: "grain-rec-00abc123",
    Title:       "My Recording",
    RecordedAt:  time.Now().UTC(),
}, utterances, grain.Options{Narrator: "Dani Roxberry"})
```

See `testdata/fixtures/grain/transcript.json` and `testdata/fixtures/recording.json` for example input/output shapes.
