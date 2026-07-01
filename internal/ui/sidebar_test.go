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

// TestWorkloadSelection covers helm-bundle drill-down: a k8s resource that
// reports multiple workloads expands into selectable child rows, and selecting a
// child filters the log pane to that workload's pods while resource actions still
// target the parent. The label grouping (apps/infra headers) must survive.
func TestWorkloadSelection(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	v := &tilt.View{
		UIResources: []tilt.UIResource{{
			Metadata: tilt.ObjectMeta{Name: "bundle", Labels: map[string]string{"apps": ""}},
			Status: tilt.UIResourceStatus{
				UpdateStatus: "ok", RuntimeStatus: "ok", Order: 1,
				K8sResourceInfo: &tilt.K8sResourceInfo{
					PodName:      "web-abc-1",
					PodStatus:    "Running",
					DisplayNames: []string{"web:deployment", "worker:deployment", "web:service"},
				},
			},
		}},
		LogList: tilt.LogList{
			Spans: map[string]tilt.LogSpan{
				"pod:bundle:web-abc-1":    {ManifestName: "bundle"},
				"pod:bundle:worker-def-2": {ManifestName: "bundle"},
			},
			Segments: []tilt.LogSegment{
				{SpanID: "pod:bundle:web-abc-1", Text: "hello from web\n"},
				{SpanID: "pod:bundle:worker-def-2", Text: "hello from worker\n"},
			},
		},
	}

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10350, Label: "app"}}})
	m = step(m, viewMsg{port: 10350, view: v})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // leave overview

	// Rows: bundle (sel 1), then its workloads web (2) and worker (3), sorted.
	if rows := m.selectableRows(); len(rows) != 3 {
		t.Fatalf("selectableRows = %d, want 3 (resource + 2 workloads)", len(rows))
	}

	m.selected = 2 // the "web" workload child
	mn, wl, ok := m.selectedWorkload()
	if !ok || mn != "bundle" || wl != "web" {
		t.Fatalf("selectedWorkload = (%q,%q,%v), want (bundle,web,true)", mn, wl, ok)
	}
	// Resource actions still target the parent release.
	if r, ok := m.selectedResource(); !ok || r.Name() != "bundle" {
		t.Fatalf("selectedResource on workload row = %q,%v, want bundle", r.Name(), ok)
	}

	m.setLogs()
	frame := ansi.Strip(m.View())
	if !strings.Contains(frame, "bundle / web") {
		t.Errorf("header missing workload title:\n%s", frame)
	}
	if !strings.Contains(frame, "hello from web") || strings.Contains(frame, "hello from worker") {
		t.Errorf("logs not filtered to the web workload:\n%s", frame)
	}
	// Grouping + nesting preserved.
	for _, want := range []string{"apps", "bundle", "└ web", "└ worker"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing %q:\n%s", want, frame)
		}
	}
}

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
