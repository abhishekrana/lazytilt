package tilt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrettifyJSON(t *testing.T) {
	dir := t.TempDir()

	// A compact JSON file is reindented in place and stays valid/equivalent.
	path := filepath.Join(dir, "snap.json")
	if err := os.WriteFile(path, []byte(`{"createdAt":"now","view":{"a":1,"b":[1,2]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	prettifyJSON(path)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\n") || !strings.Contains(string(b), "  ") {
		t.Errorf("expected indented JSON, got %q", b)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Errorf("prettified output is not valid JSON: %v", err)
	}

	// A non-JSON file is left untouched (best-effort).
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	prettifyJSON(bad)
	if b, _ := os.ReadFile(bad); string(b) != "not json" {
		t.Errorf("non-JSON file should be untouched, got %q", b)
	}
}
