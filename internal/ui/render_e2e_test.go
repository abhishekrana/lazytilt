package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// End-to-end render tests: build the model, feed it a view the way the Hub would,
// drive it like a user, and assert on the rendered frame. Fixtures use generic
// mock names only (api / worker / web; labels backend / frontend).

func e2eRes(name, label string) tilt.UIResource {
	return tilt.UIResource{
		Metadata: tilt.ObjectMeta{Name: name, Labels: map[string]string{label: ""}},
		Status:   tilt.UIResourceStatus{UpdateStatus: "ok", RuntimeStatus: "ok"},
	}
}

// drive builds a model, sizes it, feeds one instance + view, and leaves the
// overview so the sidebar and log pane are active — the state a user lands in
// after drilling into an instance.
func drive(t *testing.T, view *tilt.View) Model {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10350, Label: "app"}}})
	m = step(m, viewMsg{port: 10350, view: view})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // leave the overview into the instance
	return m
}

// A selected resource's logs must render even when the sidebar is label-grouped.
func TestSelectedResourceLogsRenderWithLabelGroups(t *testing.T) {
	view := &tilt.View{
		UIResources: []tilt.UIResource{
			e2eRes("web", "frontend"),
			e2eRes("api", "backend"),
			e2eRes("worker", "backend"),
		},
		LogList: tilt.LogList{
			Spans:    map[string]tilt.LogSpan{"s:api": {ManifestName: "api"}},
			Segments: []tilt.LogSegment{{SpanID: "s:api", Text: "api log line\n", Level: "INFO"}},
		},
	}
	m := drive(t, view)
	m.selectByName("api")
	m.setLogs()

	if r, ok := m.selectedResource(); !ok || r.Name() != "api" {
		t.Fatalf("selection did not resolve to api (ok=%v)", ok)
	}
	if frame := m.View(); !strings.Contains(frame, "api log line") {
		t.Fatalf("selected resource's log text not rendered:\n%s", frame)
	}
}

// Regression: a resource that emits CRLF ("\r\n") line endings must still render.
// sanitizeLogLine used to keep only the text after the last CR, so a line that
// merely ended with a carriage return collapsed to "" and the pane looked empty.
func TestCRLFResourceLogsRender(t *testing.T) {
	view := &tilt.View{
		UIResources: []tilt.UIResource{e2eRes("api", "backend")},
		LogList: tilt.LogList{
			Spans: map[string]tilt.LogSpan{"s:api": {ManifestName: "api"}},
			Segments: []tilt.LogSegment{
				{SpanID: "s:api", Text: "first crlf line\r\nsecond crlf line\r\n", Level: "INFO"},
			},
		},
	}
	m := drive(t, view)
	m.selectByName("api")
	m.setLogs()

	frame := m.View()
	for _, want := range []string{"first crlf line", "second crlf line"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("CRLF log line %q not rendered:\n%s", want, frame)
		}
	}
}

// New logs must keep rendering even when the total segment count is unchanged.
// Once the accumulator's history cap is reached the count stays constant while
// content rolls forward; a count-based render guard froze the pane (no scrolling).
func TestLogPaneUpdatesWhenSegmentCountUnchanged(t *testing.T) {
	mkView := func(newest string) *tilt.View {
		return &tilt.View{
			UIResources: []tilt.UIResource{e2eRes("api", "backend")},
			LogList: tilt.LogList{
				Spans: map[string]tilt.LogSpan{"s:api": {ManifestName: "api"}},
				Segments: []tilt.LogSegment{ // fixed count (2), content rolls forward
					{SpanID: "s:api", Text: "older line\n", Level: "INFO"},
					{SpanID: "s:api", Text: newest + "\n", Level: "INFO"},
				},
			},
		}
	}
	m := drive(t, mkView("first newest")) // lands in the instance, All Resources selected
	if frame := m.View(); !strings.Contains(frame, "first newest") {
		t.Fatalf("setup: first delta did not render:\n%s", frame)
	}

	// Second delta: SAME segment count, newer content (post-cap steady state).
	m = step(m, viewMsg{port: 10350, view: mkView("second newest")})
	if frame := m.View(); !strings.Contains(frame, "second newest") {
		t.Fatalf("log pane froze on a same-count delta (no scrolling):\n%s", frame)
	}
}

// Scrolling up to read history must survive an incoming delta on the same
// selection — a log delta must not yank the view back to the bottom.
func TestScrollPositionSurvivesDelta(t *testing.T) {
	apiView := func(n int) *tilt.View {
		segs := make([]tilt.LogSegment, n)
		for i := range segs {
			segs[i] = tilt.LogSegment{SpanID: "s:api", Text: fmt.Sprintf("api line %d\n", i), Level: "INFO"}
		}
		return &tilt.View{
			UIResources: []tilt.UIResource{e2eRes("api", "backend")},
			LogList:     tilt.LogList{Spans: map[string]tilt.LogSpan{"s:api": {ManifestName: "api"}}, Segments: segs},
		}
	}
	m := drive(t, apiView(50))
	m.selectByName("api")
	m.log.follow = true
	m.setLogs()
	m.log.scrollUp(20) // read history — disables follow

	if f := m.View(); strings.Contains(f, "api line 49") {
		t.Fatalf("after scroll-up the newest line should be off-screen:\n%s", f)
	}
	// A delta on the same selection must preserve the scroll position.
	m = step(m, viewMsg{port: 10350, view: apiView(51)})
	if f := m.View(); strings.Contains(f, "api line 50") {
		t.Fatalf("an incoming delta yanked the scroll position to the bottom:\n%s", f)
	}
}

// The combined "All Resources" view (selection row 0) must also render CRLF lines.
func TestCRLFLogsRenderInCombinedView(t *testing.T) {
	view := &tilt.View{
		UIResources: []tilt.UIResource{e2eRes("api", "backend")},
		LogList: tilt.LogList{
			Spans:    map[string]tilt.LogSpan{"s:api": {ManifestName: "api"}},
			Segments: []tilt.LogSegment{{SpanID: "s:api", Text: "combined crlf line\r\n", Level: "INFO"}},
		},
	}
	m := drive(t, view)
	m.selected = 0 // All Resources row
	m.setLogs()

	if frame := m.View(); !strings.Contains(frame, "combined crlf line") {
		t.Fatalf("CRLF line not rendered in combined view:\n%s", frame)
	}
}
