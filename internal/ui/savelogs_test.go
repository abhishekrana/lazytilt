package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/abhishekrana/lazytilt/internal/tilt"
)

func TestSaveLogsWritesTimestampedFile(t *testing.T) {
	v := &tilt.View{
		UIResources: []tilt.UIResource{{
			Metadata: tilt.ObjectMeta{Name: "api"},
			Status:   tilt.UIResourceStatus{UpdateStatus: "ok", Order: 1},
		}},
		LogList: tilt.LogList{
			Spans: map[string]tilt.LogSpan{"s": {ManifestName: "api"}},
			Segments: []tilt.LogSegment{
				{SpanID: "s", Text: "line one\n", Level: "INFO"},
				{SpanID: "s", Text: "\x1b[31mred error\x1b[0m\n", Level: "ERROR"},
				{SpanID: "s", Text: "progress\rfinal\n", Level: "INFO"},
			},
		},
	}
	m := detailView(t, v)

	// Plain text: all levels, ANSI stripped, carriage-return overwrite resolved.
	text := m.resourceLogText(v.UIResources[0])
	if want := "line one\nred error\nfinal\n"; text != want {
		t.Fatalf("resourceLogText = %q, want %q", text, want)
	}

	msg, ok := saveLogsCmd("api", text)().(notifyMsg)
	if !ok || msg.err {
		t.Fatalf("save failed: %+v", msg)
	}
	path := strings.TrimPrefix(msg.text, "saved logs → ")
	t.Cleanup(func() { os.Remove(path) })

	base := path[strings.LastIndexByte(path, '/')+1:]
	if !strings.HasPrefix(base, "lazytilt-api-") || !strings.HasSuffix(base, ".log") {
		t.Errorf("unexpected filename %q", base)
	}
	// A YYYYMMDD-HHMMSS stamp adds the digits between name and random suffix.
	if strings.Count(base, "-") < 3 {
		t.Errorf("filename %q does not look timestamped", base)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(b) != text {
		t.Errorf("file content = %q, want %q", b, text)
	}
}
