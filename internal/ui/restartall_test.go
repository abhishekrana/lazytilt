package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestRestartAllConfirmation(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "")
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 26})
	m = step(m, twoInstances()) // active instance is app-one :10350
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc}) // leave the overview

	// Targets exclude the (Tiltfile) pseudo-resource and the disabled batch-job,
	// leaving api, worker, db.
	if got := m.restartTargets(); len(got) != 3 {
		t.Fatalf("restartTargets = %v, want 3 (api, worker, db)", got)
	}
	for _, skip := range []string{"(Tiltfile)", "batch-job"} {
		for _, n := range m.restartTargets() {
			if n == skip {
				t.Errorf("restartTargets should not include %q", skip)
			}
		}
	}

	// R asks to confirm and does NOT act yet.
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if m.mode != modeConfirm || !m.pendingRestartAll {
		t.Fatalf("R should open a restart-all confirmation; mode=%v pending=%v", m.mode, m.pendingRestartAll)
	}
	if frame := ansi.Strip(m.View()); !strings.Contains(frame, "Restart all 3 resources in app-one?") {
		t.Errorf("confirmation prompt missing/!= expected:\n%s", frame)
	}
	if m.statusMsg != "" {
		t.Error("nothing should run before confirmation")
	}

	// n cancels cleanly.
	if c := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}); c.mode != modeNormal || c.pendingRestartAll {
		t.Error("n should cancel the restart-all without acting")
	}

	// y confirms and kicks off the batch.
	y := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if y.mode != modeNormal || y.pendingRestartAll {
		t.Error("y should dismiss the confirmation")
	}
	if !strings.Contains(y.statusMsg, "restarting 3 resources") {
		t.Errorf("expected a restart status, got %q", y.statusMsg)
	}
}
