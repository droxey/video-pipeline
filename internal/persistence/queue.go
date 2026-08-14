package persistence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nebula/course-video-pipeline/internal/domain"
)

const reviewQueueFile = "review_queue.jsonl"

// ReviewQueuePath returns the canonical path for the run's human review queue.
func ReviewQueuePath(runDir string) string {
	return filepath.Join(runDir, "review", reviewQueueFile)
}

// AppendReviewEntry appends entry as a single JSON line to the review queue at path.
// The file and its parent directory are created if they do not exist.
func AppendReviewEntry(path string, entry *domain.ReviewQueueEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("queue: mkdir: %w", err)
	}
	return appendJSONL(path, entry)
}

// ReadReviewQueue reads and returns all entries from the JSONL queue at path.
// Returns (nil, nil) when the file does not exist.
func ReadReviewQueue(path string) ([]*domain.ReviewQueueEntry, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("queue: open %s: %w", path, err)
	}
	defer f.Close()
	return scanJSONL[domain.ReviewQueueEntry](path, f)
}

// ResolveReviewEntry finds the entry with the given id, sets its status and optional
// reviewer notes, then rewrites the queue file atomically. Returns an error when the
// id is not found or the file cannot be read/written.
func ResolveReviewEntry(path, id string, status domain.ReviewQueueStatus, notes string) error {
	entries, err := ReadReviewQueue(path)
	if err != nil {
		return err
	}

	found := false
	for _, e := range entries {
		if e.ID == id {
			e.Status = status
			if notes != "" {
				e.ReviewerNotes = &notes
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("queue: entry %q not found in %s", id, path)
	}

	// Rewrite atomically.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-queue-")
	if err != nil {
		return fmt.Errorf("queue: create temp: %w", err)
	}
	tmpName := tmp.Name()

	w := bufio.NewWriter(tmp)
	for _, e := range entries {
		data, merr := json.Marshal(e)
		if merr != nil {
			tmp.Close()
			os.Remove(tmpName)
			return fmt.Errorf("queue: marshal entry %s: %w", e.ID, merr)
		}
		w.Write(data) //nolint:errcheck // buffered; final flush catches errors
		w.WriteByte('\n')
	}
	if ferr := w.Flush(); ferr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("queue: flush: %w", ferr)
	}
	if serr := tmp.Sync(); serr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("queue: sync: %w", serr)
	}
	tmp.Close()
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("queue: rename: %w", rerr)
	}
	return nil
}

// scanJSONL reads newline-delimited JSON objects from r, decoding each into T.
// path is used only in error messages.
func scanJSONL[T any](path string, r io.Reader) ([]*T, error) {
	var results []*T
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(line, &v); err != nil {
			return nil, fmt.Errorf("queue: parse line in %s: %w", path, err)
		}
		results = append(results, &v)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("queue: scan %s: %w", path, err)
	}
	return results, nil
}
