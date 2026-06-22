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
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})  // leave the overview to see the log pane
	m = step(m, tea.KeyMsg{Type: tea.KeyDown}) // select the resource (index 0 is All Resources)

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

// TestAllResourcesCombinedView checks the synthetic "All Resources" row (the
// default selection): logs from every resource plus global Tilt output, in
// order, each line tagged with its source.
func TestAllResourcesCombinedView(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	v := &tilt.View{
		UIResources: []tilt.UIResource{
			{Metadata: tilt.ObjectMeta{Name: "api"}, Status: tilt.UIResourceStatus{UpdateStatus: "ok", Order: 1}},
			{Metadata: tilt.ObjectMeta{Name: "worker"}, Status: tilt.UIResourceStatus{UpdateStatus: "error", Order: 2}},
		},
		LogList: tilt.LogList{
			Spans: map[string]tilt.LogSpan{
				"g": {ManifestName: ""}, // global / Tiltfile output
				"a": {ManifestName: "api"},
				"w": {ManifestName: "worker"},
			},
			Segments: []tilt.LogSegment{
				{SpanID: "g", Text: "tilt starting\n", Level: "INFO"},
				{SpanID: "a", Text: "loading\rapi up\n", Level: "INFO"}, // carriage-return overwrite
				{SpanID: "w", Text: "worker boom\n", Level: "ERROR"},
			},
		},
	}
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10350, Label: "app"}}})
	m = step(m, viewMsg{port: 10350, view: v})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // lands on the All Resources row (index 0)

	if !m.onAllLogs() {
		t.Fatal("default selection should be the All Resources row")
	}
	frame := ansi.Strip(m.View())

	if !strings.Contains(frame, "All Resources") || !strings.Contains(frame, "2 resources") {
		t.Errorf("combined header missing:\n%s", frame)
	}
	for _, want := range []string{"tilt starting", "api up", "worker boom"} {
		if !strings.Contains(frame, want) {
			t.Errorf("combined view missing %q", want)
		}
	}
	// Carriage-return overwrite resolved even in the combined stream.
	if strings.Contains(frame, "loading") {
		t.Error("overwritten text should not appear in the combined stream")
	}
	// Interleaved in segment order: global, then api, then worker.
	gi, ai, wi := strings.Index(frame, "tilt starting"), strings.Index(frame, "api up"), strings.Index(frame, "worker boom")
	if !(gi < ai && ai < wi) {
		t.Errorf("combined order wrong: tilt=%d api=%d worker=%d", gi, ai, wi)
	}
}
