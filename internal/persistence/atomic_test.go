package persistence_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nebula/course-video-pipeline/internal/persistence"
)

type sample struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestWriteJSON_ReadJSON_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	in := sample{Name: "nebula", Value: 42}
	if err := persistence.WriteJSON(path, in); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var out sample
	if err := persistence.ReadJSON(path, &out); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if out.Name != in.Name || out.Value != in.Value {
		t.Errorf("round-trip mismatch: got %+v, want %+v", out, in)
	}
}

func TestWriteJSON_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	// Write once.
	if err := persistence.WriteJSON(path, sample{Name: "first"}); err != nil {
		t.Fatal(err)
	}
	// Overwrite atomically.
	if err := persistence.WriteJSON(path, sample{Name: "second", Value: 99}); err != nil {
		t.Fatalf("WriteJSON overwrite: %v", err)
	}

	var out sample
	persistence.ReadJSON(path, &out)
	if out.Name != "second" {
		t.Errorf("expected overwritten value, got %q", out.Name)
	}
}

func TestWriteJSON_MissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "data.json")
	err := persistence.WriteJSON(path, sample{Name: "x"})
	if err == nil {
		t.Error("expected error writing to missing directory")
	}
}

func TestReadJSON_NotFound(t *testing.T) {
	var out sample
	err := persistence.ReadJSON("/nonexistent/path/data.json", &out)
	if err == nil {
		t.Error("expected error reading missing file")
	}
}

func TestReadJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not valid json {{"), 0o644)
	var out sample
	if err := persistence.ReadJSON(path, &out); err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestWriteJSON_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.json")
	persistence.WriteJSON(path, sample{Name: "clean"})

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "clean.json" {
			t.Errorf("unexpected file left behind: %q", e.Name())
		}
	}
}
