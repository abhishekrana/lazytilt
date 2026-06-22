package ui

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotPath(t *testing.T) {
	base := filepath.Base(snapshotPath("app one/two")) // space + slash must be sanitized

	if !strings.HasPrefix(base, "lazytilt-snapshot-") || !strings.HasSuffix(base, ".json") {
		t.Errorf("unexpected snapshot filename %q", base)
	}
	if strings.ContainsAny(base, " /") {
		t.Errorf("filename not filesystem-safe: %q", base)
	}
	if !strings.Contains(base, "app-one-two") {
		t.Errorf("label %q not sanitized into the filename", base)
	}
	// A YYYYMMDD-HHMMSS stamp adds the digits between label and extension.
	if strings.Count(base, "-") < 4 {
		t.Errorf("filename %q does not look timestamped", base)
	}
}
