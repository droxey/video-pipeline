// Package persistence handles atomic writes, manifests, approval records, and the review queue.
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteJSON atomically writes v as indented JSON to path.
// It writes to a sibling temp file then renames, so readers never see a partial file.
// The destination directory must already exist.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence: marshal for %s: %w", path, err)
	}
	data = append(data, '\n')
	return writeAtomic(path, data)
}

// ReadJSON reads path and JSON-decodes it into v.
func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("persistence: read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("persistence: unmarshal %s: %w", path, err)
	}
	return nil
}

// appendJSONL marshals v and appends it as a single newline-terminated JSON line to path.
// The file is created if it does not exist.
func appendJSONL(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("persistence: marshal JSONL for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("persistence: open %s: %w", path, err)
	}
	defer f.Close()
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("persistence: write %s: %w", path, err)
	}
	return nil
}

// writeAtomic writes data to a temp file in the same directory as path, then renames.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-atomic-")
	if err != nil {
		return fmt.Errorf("persistence: create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	if _, werr := tmp.Write(data); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("persistence: write temp %s: %w", tmpName, werr)
	}
	if serr := tmp.Sync(); serr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("persistence: sync temp %s: %w", tmpName, serr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("persistence: close temp %s: %w", tmpName, cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("persistence: rename %s → %s: %w", tmpName, path, rerr)
	}
	return nil
}
