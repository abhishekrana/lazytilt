package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestSolarizedColorsRendered forces a truecolor profile and asserts the frame
// emits the Solarized Light foreground palette (blue accent, base01 text). We
// don't paint a background, so only foreground escapes are expected.
func TestSolarizedColorsRendered(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "solarized-light")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})

	frame := m.View()
	for _, want := range []string{
		"38;2;38;139;210", // blue accent (#268bd2) — header title + rule
		"38;2;88;110;117", // base01 text (#586e75)
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing Solarized foreground escape %q", want)
		}
	}
}
