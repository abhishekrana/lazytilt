package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestThemeRegistry(t *testing.T) {
	if got := resolveTheme("").Name; got != defaultTheme {
		t.Errorf("empty theme = %q, want default %q", got, defaultTheme)
	}
	if got := resolveTheme("dark").Name; got != "dark" {
		t.Errorf("dark lookup = %q", got)
	}
	if got := resolveTheme("does-not-exist").Name; got != defaultTheme {
		t.Errorf("unknown theme = %q, want fallback to %q", got, defaultTheme)
	}
	if resolveTheme("").Name != "solarized-light" {
		t.Error("default theme should be solarized-light")
	}
}

func TestThemeCycleWraps(t *testing.T) {
	names := make([]string, 0, len(themeOrder)+1)
	th := resolveTheme("")
	for i := 0; i <= len(themeOrder); i++ {
		names = append(names, th.Name)
		th = th.next()
	}
	if names[0] != names[len(themeOrder)] {
		t.Errorf("cycle did not wrap to start: %v", names)
	}
	if names[0] != "solarized-light" || names[1] != "solarized-dark" || names[2] != "dark" {
		t.Errorf("cycle order = %v", names[:len(themeOrder)])
	}
}

func TestNewAppliesThemeAndTCycles(t *testing.T) {
	m := New("", "localhost", 10350, "solarized-dark")
	if m.theme.Name != "solarized-dark" {
		t.Fatalf("New theme = %q, want solarized-dark", m.theme.Name)
	}
	nm, _ := m.updateKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if got := nm.(Model).theme.Name; got == "solarized-dark" {
		t.Error("T should cycle to a different theme")
	}
}
