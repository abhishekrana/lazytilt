package ui

import (
	"strings"
	"testing"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestHighlightMatches(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	style := lipgloss.NewStyle().Reverse(true)
	got := highlightMatches("Hello WORLD hello", "hello", style)

	if ansi.Strip(got) != "Hello WORLD hello" {
		t.Errorf("visible text changed: %q", ansi.Strip(got))
	}
	if got == "Hello WORLD hello" {
		t.Error("expected highlight styling to be applied")
	}
	if strings.Count(got, "\x1b[7m") != 2 {
		t.Errorf("expected 2 highlighted (reverse) spans, got %d", strings.Count(got, "\x1b[7m"))
	}
	// Original casing preserved for both matches.
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "hello") {
		t.Error("original casing should be preserved")
	}
}

func TestSanitizeLogLine(t *testing.T) {
	cases := map[string]string{
		"plain":             "plain",
		"a\rb\rc":           "c",                 // carriage returns: keep the final overwrite
		"progress\r":        "",                  // trailing \r leaves nothing
		"tab\tsep":          "tab  sep",          // tab -> spaces
		"x\x08y":            "xy",                // other C0 controls dropped
		"\x1b[32mok\x1b[0m": "\x1b[32mok\x1b[0m", // ESC/SGR preserved
	}
	for in, want := range cases {
		if got := sanitizeLogLine(in); got != want {
			t.Errorf("sanitizeLogLine(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCarriageReturnLogsDoNotCorrupt reproduces curl/progress output (which uses
// \r to redraw in place) and verifies it can't bleed into the layout.
func TestCarriageReturnLogsDoNotCorrupt(t *testing.T) {
	v := &tilt.View{
		UIResources: []tilt.UIResource{{
			Metadata: tilt.ObjectMeta{Name: "svc"},
			Status:   tilt.UIResourceStatus{UpdateStatus: "ok", RuntimeStatus: "ok"},
		}},
		LogList: tilt.LogList{
			Spans:    map[string]tilt.LogSpan{"s": {ManifestName: "svc"}},
			Segments: []tilt.LogSegment{{SpanID: "s", Text: "progress 1\rprogress 2\rfinal line\n", Level: "INFO"}},
		},
	}
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10350, Label: "app"}}})
	m = step(m, viewMsg{port: 10350, view: v})

	frame := m.View()
	if strings.ContainsRune(frame, '\r') {
		t.Error("rendered frame still contains a carriage return")
	}
	if !strings.Contains(frame, "final line") {
		t.Error("expected the post-\\r text 'final line' to render")
	}
	if strings.Contains(frame, "progress 1") {
		t.Error("overwritten progress text should not appear")
	}
}
