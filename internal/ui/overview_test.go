package ui

import (
	"strings"
	"testing"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// failCompose loads the compose fixture and forces one resource into an error
// state, so the overview has a non-OK compose (not k8s) row to render.
func failCompose(t *testing.T) *tilt.View {
	t.Helper()
	v := mustView(t, "view_compose.json")
	for i := range v.UIResources {
		if v.UIResources[i].Name() == "web" {
			v.UIResources[i].Status.RuntimeStatus = "error"
		}
	}
	return v
}

func TestTopBarHealthBadges(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")}) // 1 error
	m = step(m, viewMsg{port: 10360, view: mustView(t, "view_compose.json")})

	frame := ansi.Strip(m.View())
	if !strings.Contains(frame, "✕1") {
		t.Errorf("top bar should badge the k8s instance with ✕1:\n%s", frame)
	}
	if !strings.Contains(frame, "‹0› overview") {
		t.Error("top bar should offer the ‹0› overview tab")
	}
}

func TestOverviewAcrossBackends(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = step(m, twoInstances()) // ‹1› app-one :10350, ‹2› app-two :10360
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	m = step(m, viewMsg{port: 10360, view: failCompose(t)})

	if !m.overview {
		t.Fatal("the overview should be the starting screen")
	}

	frame := ansi.Strip(m.View())
	// Both backends' failing resources show in one place, with their backend label.
	for _, want := range []string{"OVERVIEW", "app-one", "app-two", "worker", "k8s", "web", "compose", "✕"} {
		if !strings.Contains(frame, want) {
			t.Errorf("overview missing %q:\n%s", want, frame)
		}
	}
	// Healthy resources are collapsed into the counts, not listed as rows.
	if strings.Contains(frame, " db ") {
		t.Error("an OK resource (db) should not be listed as an overview row")
	}
}

func TestOverviewEnterJumpsToResource(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	m = step(m, viewMsg{port: 10360, view: failCompose(t)})

	// Overview is the landing screen. rows: [0] app-one header, [1] worker
	// (its only error). Select worker, then open it.
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.overview {
		t.Error("enter should leave the overview")
	}
	if m.currentPort() != 10350 {
		t.Errorf("should land on app-one (:10350), got :%d", m.currentPort())
	}
	if r, ok := m.selectedResource(); !ok || r.Name() != "worker" {
		t.Errorf("worker should be selected, got %q (ok=%v)", r.Name(), ok)
	}
	if m.focus != focusLogs {
		t.Error("focus should move to the logs after jumping to a resource")
	}
}

func TestOverviewIsLandingScreenAndToggles(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})

	if !m.overview {
		t.Fatal("the app should start on the overview")
	}
	if !strings.Contains(ansi.Strip(m.View()), "OVERVIEW") {
		t.Error("landing frame should be the overview")
	}

	// esc drills into the single-instance view; 0 returns to the overview.
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.overview {
		t.Error("esc should leave the overview")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	if !m.overview {
		t.Error("0 should reopen the overview")
	}
}

func TestOverviewOnlyFailingHidesHealthy(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})     // has an error
	m = step(m, viewMsg{port: 10360, view: mustView(t, "view_compose.json")}) // all healthy

	// Overview is the landing screen.
	// The port appears only in the overview header, never in the top-bar tabs.
	if before := ansi.Strip(m.View()); !strings.Contains(before, ":10360") {
		t.Fatalf("healthy instance header should be present before only-failing:\n%s", before)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	after := ansi.Strip(m.View())
	if strings.Contains(after, ":10360") {
		t.Error("only-failing should hide the all-healthy instance's block")
	}
	if !strings.Contains(after, ":10350") {
		t.Error("the failing instance should remain under only-failing")
	}
}
