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

func TestSidebarLabelGrouping(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	v := &tilt.View{UIResources: []tilt.UIResource{
		{Metadata: tilt.ObjectMeta{Name: "zebra", Labels: map[string]string{"alpha": ""}}, Status: tilt.UIResourceStatus{UpdateStatus: "ok", Order: 1}},
		{Metadata: tilt.ObjectMeta{Name: "apple", Labels: map[string]string{"alpha": ""}}, Status: tilt.UIResourceStatus{UpdateStatus: "error", Order: 2}},
		{Metadata: tilt.ObjectMeta{Name: "gamma", Labels: map[string]string{"beta": ""}}, Status: tilt.UIResourceStatus{UpdateStatus: "ok", Order: 3}},
		{Metadata: tilt.ObjectMeta{Name: "delta"}, Status: tilt.UIResourceStatus{UpdateStatus: "ok", Order: 4}},
	}}
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10350, Label: "app"}}})
	m = step(m, viewMsg{port: 10350, view: v})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // drill into the instance

	// Grouped by label (alphabetical), "(no label)" last, resources alphabetical
	// within a group: alpha[apple, zebra], beta[gamma], (no label)[delta].
	var order []string
	for _, r := range m.visible() {
		order = append(order, r.Name())
	}
	if got := strings.Join(order, ","); got != "apple,zebra,gamma,delta" {
		t.Errorf("visible order = %q, want apple,zebra,gamma,delta", got)
	}

	frame := ansi.Strip(m.View())
	for _, h := range []string{"alpha", "beta", "(no label)"} {
		if !strings.Contains(frame, h) {
			t.Errorf("sidebar missing group header %q:\n%s", h, frame)
		}
	}
	// Header rollup: the alpha group has one errored resource.
	if !strings.Contains(frame, "✕1") {
		t.Errorf("alpha header should show a ✕1 rollup:\n%s", frame)
	}

	// Selection skips headers: the first Down off "All Resources" lands on the
	// first resource (apple), not the "alpha" header.
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if r, ok := m.selectedResource(); !ok || r.Name() != "apple" {
		t.Fatalf("first Down should select apple, got %q (ok=%v)", r.Name(), ok)
	}
	// Selection clamps at the last resource regardless of the headers in between.
	for range 5 {
		m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if r, ok := m.selectedResource(); !ok || r.Name() != "delta" {
		t.Errorf("selection should clamp at delta, got %q (ok=%v)", r.Name(), ok)
	}
}
