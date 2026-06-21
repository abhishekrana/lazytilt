package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
)

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
	// Select the failing k8s resource (index 2: (Tiltfile), api, worker).
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})

	frame := m.View()
	for _, want := range []string{"LAZYTILT", "app-one", "app-two", "api", "worker", "k8s", "pod pending", "✕"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing %q", want)
		}
	}
	// Disabled resource hidden by default.
	if strings.Contains(frame, "batch-job") {
		t.Error("disabled resource should be hidden by default")
	}
	t.Logf("\n%s", frame)
}

func TestRenderCompose(t *testing.T) {
	m := New("", "localhost", 10360, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10360, Label: "app-two"}}})
	m = step(m, viewMsg{port: 10360, view: mustView(t, "view_compose.json")})
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
	m = step(m, twoInstances()) // ‹1› app-one :10350 (active), ‹2› app-two :10360
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	if m.currentPort() != 10350 {
		t.Fatalf("start port = %d, want 10350", m.currentPort())
	}

	// Pressing "2" jumps directly to the second instance.
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.currentPort() != 10360 {
		t.Errorf("after '2' port = %d, want 10360", m.currentPort())
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
