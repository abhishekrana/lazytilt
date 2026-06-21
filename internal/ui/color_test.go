package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestSolarizedColorsRendered forces a truecolor profile and asserts the frame
// actually emits the Solarized Light palette (background base3, blue accent).
func TestSolarizedColorsRendered(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := New("", "localhost", 10350, "solarized-light")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = step(m, twoInstances())
	m = step(m, viewMsg{port: 10350, view: mustView(t, "view_k8s.json")})

	frame := m.View()
	for _, want := range []string{
		"48;2;253;246;227", // base3 background (#fdf6e3)
		"38;2;38;139;210",  // blue accent (#268bd2)
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame missing Solarized escape %q", want)
		}
	}
}
