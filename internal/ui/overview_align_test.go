package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// mkView builds a synthetic view with ok healthy resources, padded out to total
// with idle ones, and an optional failing resource of the given backend.
func mkView(ok, total int, errName, backend string) *tilt.View {
	v := &tilt.View{}
	ord := 0
	for range ok {
		v.UIResources = append(v.UIResources, tilt.UIResource{
			Metadata: tilt.ObjectMeta{Name: fmt.Sprintf("svc-%d", ord)},
			Status:   tilt.UIResourceStatus{UpdateStatus: "ok", Order: ord},
		})
		ord++
	}
	for ord < total {
		v.UIResources = append(v.UIResources, tilt.UIResource{
			Metadata: tilt.ObjectMeta{Name: fmt.Sprintf("idle-%d", ord)},
			Status:   tilt.UIResourceStatus{Order: ord},
		})
		ord++
	}
	if errName != "" {
		st := tilt.UIResourceStatus{RuntimeStatus: "error", Order: ord}
		if backend == "compose" {
			st.Compose = &tilt.ComposeResourceInfo{HealthStatus: "unhealthy"}
		}
		v.UIResources = append(v.UIResources, tilt.UIResource{
			Metadata: tilt.ObjectMeta{Name: errName},
			Status:   st,
		})
	}
	return v
}

// TestOverviewColumnsAlign guards the overview grid: every instance header's
// port must start at the same column (regardless of name length), and a failing
// resource's detail must line up under that same port column.
func TestOverviewColumnsAlign(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = step(m, instancesMsg{instances: []discovery.Instance{
		{Host: "localhost", Port: 10350, Label: "cloud"},
		{Host: "localhost", Port: 10360, Label: "data-farm"},
		{Host: "localhost", Port: 10370, Label: "onprem"},
		{Host: "localhost", Port: 10390, Label: "benchmarking"},
	}})
	m = step(m, viewMsg{port: 10350, view: mkView(11, 11, "", "")})
	m = step(m, viewMsg{port: 10360, view: mkView(19, 20, "", "")})
	m = step(m, viewMsg{port: 10370, view: mkView(11, 12, "prometheus-federated", "compose")})
	m = step(m, viewMsg{port: 10390, view: mkView(2, 3, "", "")})

	lines := strings.Split(ansi.Strip(m.View()), "\n")

	// displayCol is the on-screen column where sub begins (counting display cells,
	// not bytes — ▶ and ‹› are multi-byte runes that occupy one cell each).
	displayCol := func(line, sub string) int {
		before, _, found := strings.Cut(line, sub)
		if !found {
			return -1
		}
		return lipgloss.Width(before)
	}

	portCols := map[int]bool{}
	nPorts := 0
	for _, ln := range lines {
		if c := displayCol(ln, ":103"); c >= 0 {
			portCols[c] = true
			nPorts++
		}
	}
	if nPorts != 4 {
		t.Fatalf("expected 4 instance headers, found %d", nPorts)
	}
	if len(portCols) != 1 {
		t.Errorf("instance ports are not column-aligned; start columns: %v", portCols)
	}
	portCol := -1
	for c := range portCols {
		portCol = c
	}

	// The failing resource's detail aligns under the port column.
	for _, ln := range lines {
		if strings.Contains(ln, "prometheus-federated") {
			if c := displayCol(ln, "compose"); c != portCol {
				t.Errorf("resource detail at col %d, want %d (under the port column)", c, portCol)
			}
		}
	}
}

// TestOverviewLongNameNotTruncated guards the name column auto-sizing: a resource
// name longer than the fixed floor must render in full with a gutter before its
// detail, not get clipped and butt straight against "compose" (the overlap bug).
func TestOverviewLongNameNotTruncated(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	const longName = "dual-franka-robot-state-publisher" // 33 cells, well past ovNameW
	v := &tilt.View{UIResources: []tilt.UIResource{{
		Metadata: tilt.ObjectMeta{Name: longName},
		Status: tilt.UIResourceStatus{
			UpdateStatus: "in_progress", // building => shown as a resource row
			Compose:      &tilt.ComposeResourceInfo{},
		},
	}}}

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 160, Height: 30})
	m = step(m, instancesMsg{instances: []discovery.Instance{
		{Host: "localhost", Port: 10350, Label: "data-farm"},
	}})
	m = step(m, viewMsg{port: 10350, view: v})

	out := ansi.Strip(m.View())
	if !strings.Contains(out, longName) {
		t.Fatalf("long resource name was truncated; frame:\n%s", out)
	}
	// A full-width name must still be followed by a gutter, never the detail directly.
	if !strings.Contains(out, longName+"  ") || strings.Contains(out, longName+" compose") {
		t.Errorf("missing gutter between name and detail (overlap); frame:\n%s", out)
	}
}
