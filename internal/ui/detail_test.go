package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
)

// detailView builds a model on a synthetic instance and selects its first
// resource, with the always-on detail strip showing.
func detailView(t *testing.T, v *tilt.View) Model {
	t.Helper()
	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, instancesMsg{instances: []discovery.Instance{{Host: "localhost", Port: 10350, Label: "app"}}})
	m = step(m, viewMsg{port: 10350, view: v})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})  // leave the overview landing screen
	m = step(m, tea.KeyMsg{Type: tea.KeyDown}) // index 0 is "All Resources"; select the first resource
	return m
}

func TestDetailStripSurfacesFetchedFields(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	v := &tilt.View{UIResources: []tilt.UIResource{{
		Metadata: tilt.ObjectMeta{Name: "api", Labels: map[string]string{"backend": "", "tier1": ""}},
		Status: tilt.UIResourceStatus{
			UpdateStatus:    "error",
			RuntimeStatus:   "ok",
			Order:           1,
			EndpointLinks:   []tilt.Link{{URL: "http://localhost:8080"}},
			K8sResourceInfo: &tilt.K8sResourceInfo{PodName: "api-7f9b9c", PodStatus: "Running", PodRestarts: 2},
			BuildHistory: []tilt.BuildTerminated{{
				Error:      "dial tcp 127.0.0.1:5432: connection refused",
				Warnings:   []string{"deprecated base image", "no health check defined"},
				StartTime:  start,
				FinishTime: start.Add(1200 * time.Millisecond),
			}},
		},
	}}}

	// The strip is always on — no keypress needed beyond leaving the overview.
	m := detailView(t, v)
	frame := m.View()
	t.Logf("\n%s", frame)

	for _, want := range []string{
		"api", "k8s", "pod api-7f9b9c", "restarts 2", "error", // header
		"build ", "1.2s", // build duration
		"http://localhost:8080", // endpoint (display only)
		"labels", "backend", "tier1",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("detail strip missing %q", want)
		}
	}

	// A failing resource surfaces its build-error summary in the strip, so the ✕
	// says what broke — the running pod's own logs otherwise bury it.
	if !strings.Contains(frame, "connection refused") {
		t.Error("last build error should appear in the detail strip")
	}

	// Build warnings are surfaced too, tagged with the count (2 here), first shown.
	if !strings.Contains(frame, "warn 2") || !strings.Contains(frame, "deprecated base image") {
		t.Error("build warnings should appear in the detail strip with a count")
	}
}
