package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestFocusIndicatorChangesFrame(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // leave the overview landing screen

	sidebarFocused := m.View()
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	logsFocused := m.View()

	if sidebarFocused == logsFocused {
		t.Error("focus change should be visually reflected in the frame")
	}
}

func mustView(t *testing.T, name string) *tilt.View {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "tilt", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	v, err := tilt.ParseView(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return v
}

func step(m Model, msg tea.Msg) Model {
	nm, _ := m.Update(msg)
	return nm.(Model)
}

func twoInstances() instancesMsg {
	return instancesMsg{instances: []discovery.Instance{
		{Host: "localhost", Port: 10350, Label: "app-one"},
		{Host: "localhost", Port: 10360, Label: "app-two"},
	}}
}

func TestRenderFrame(t *testing.T) {
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // leave the overview landing screen
	// Select the failing k8s resource (index 2: (Tiltfile), api, worker).
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})

	frame := m.View()
	for _, want := range []string{"LAZYTILT", "app-one", "app-two", "api", "worker", "k8s", "pod pending", "✕"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing %q", want)
		}
	}
	// Disabled resources are always shown now (so you can select and re-enable one).
	if !strings.Contains(frame, "batch-job") {
		t.Error("disabled resource should be visible")
	}
	t.Logf("\n%s", frame)
}

func TestRenderCompose(t *testing.T) {
	m := New("", "localhost", 10360, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10360, Label: "app-two"}}})
	m = step(m, viewMsg{port: 10360, view: mustView(t, "view_compose.json")})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // leave the overview landing screen
	// Select web (index 1).
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})

	frame := m.View()
	for _, want := range []string{"web", "compose", "container · healthy"} {
		if !strings.Contains(frame, want) {
			t.Errorf("compose frame missing %q", want)
		}
	}
	t.Logf("\n%s", frame)
}

func TestSwitchInstanceResetsView(t *testing.T) {
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	if m.view == nil {
		t.Fatal("expected a loaded view")
	}

	nm, _ := m.switchInstance(1)
	m = nm.(Model)
	if m.currentPort() != 10360 {
		t.Errorf("after switch port = %d, want 10360", m.currentPort())
	}
	if m.view != nil {
		t.Error("view should reset to nil on instance switch (no restart, fresh fetch)")
	}
}

func TestNumberKeySwitchesInstance(t *testing.T) {
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, twoInstances()) // ‹1› overview, ‹2› app-one :10350 (active), ‹3› app-two :10360
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	if m.currentPort() != 10350 {
		t.Fatalf("start port = %d, want 10350", m.currentPort())
	}

	// Instances start at ‹2›, so "3" jumps directly to the second instance.
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if m.currentPort() != 10360 {
		t.Errorf("after '3' port = %d, want 10360", m.currentPort())
	}
	if m.view != nil {
		t.Error("view should reset on switch (no restart, fresh fetch)")
	}

	// A digit beyond the instance count is a no-op.
	m = step(m, viewMsg{port: 10360, view: mustView(t, "view_compose.json")})
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")})
	if m.currentPort() != 10360 || m.view == nil {
		t.Error("'9' with two instances should be a no-op")
	}
}

func TestHelpPopupOverlay(t *testing.T) {
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})

	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	frame := m.View()
	if !strings.Contains(frame, "lazytilt — keys") {
		t.Error("help popup content missing")
	}
	// It's an overlay, not a takeover: the header is still visible behind it.
	if !strings.Contains(frame, "LAZYTILT") {
		t.Error("background frame should remain visible behind the popup")
	}

	// esc closes it.
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(m.View(), "lazytilt — keys") {
		t.Error("esc should close the help popup")
	}
}

func TestTriggerConfirmation(t *testing.T) {
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})  // leave the overview landing screen
	m = step(m, tea.KeyMsg{Type: tea.KeyDown}) // select "api" (index 1)

	// r opens a confirmation and does NOT act yet.
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.mode != modeConfirm || m.pendingResource != "api" {
		t.Fatalf("r should ask to confirm 'api', got mode=%v res=%q", m.mode, m.pendingResource)
	}
	if !strings.Contains(m.View(), "Trigger api?") {
		t.Error("confirmation popup not shown")
	}
	if m.statusMsg != "" {
		t.Error("no action should have run before confirmation")
	}

	// n cancels.
	if c := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}); c.mode != modeNormal || c.statusMsg != "" {
		t.Error("n should cancel without acting")
	}

	// y confirms and kicks off the trigger.
	y := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if y.mode != modeNormal || !strings.Contains(y.statusMsg, "trigger api") {
		t.Errorf("y should confirm; mode=%v status=%q", y.mode, y.statusMsg)
	}
}

func TestStaleViewDropped(t *testing.T) {
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, twoInstances())
	// A response for 10360 while 10350 is active must be ignored.
	m = step(m, viewMsg{port: 10360, view: mustView(t, "view_compose.json")})
	if m.view != nil {
		t.Error("stale view from non-active instance should be dropped")
	}
}
