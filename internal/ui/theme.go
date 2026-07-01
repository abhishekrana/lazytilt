package ui

import (
	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/charmbracelet/lipgloss"
)

// Theme is a named color palette. Foreground colors are used for inline text
// fragments; Bg / TopBarBg / Highlight fill whole regions so a light theme reads
// as light regardless of the terminal's own background.
type Theme struct {
	Name string

	Bg        lipgloss.Color // app background (empty = terminal default)
	TopBarBg  lipgloss.Color // instance bar background
	Highlight lipgloss.Color // selected row background
	Text      lipgloss.Color // primary text
	Muted     lipgloss.Color // secondary text
	Accent    lipgloss.Color // active instance, selection marker, backend label
	Border    lipgloss.Color

	OK       lipgloss.Color
	Err      lipgloss.Color
	Building lipgloss.Color
	Pending  lipgloss.Color
	Disabled lipgloss.Color
	Idle     lipgloss.Color
}

// defaultTheme is used when no theme is named or an unknown one is requested.
const defaultTheme = "solarized-light"

// themeOrder is the cycle order for the in-app theme switch.
var themeOrder = []string{"solarized-light", "solarized-dark", "dark"}

// themes is the registry of available palettes.
var themes = map[string]Theme{
	// https://ethanschoonover.com/solarized/
	"solarized-light": {
		Name:      "solarized-light",
		Bg:        "#fdf6e3", // base3
		TopBarBg:  "#eee8d5", // base2
		Highlight: "#eee8d5", // base2
		Text:      "#586e75", // base01
		Muted:     "#93a1a1", // base1
		Accent:    "#268bd2", // blue
		Border:    "#93a1a1", // base1
		OK:        "#859900", // green
		Err:       "#dc322f", // red
		Building:  "#cb4b16", // orange
		Pending:   "#b58900", // yellow
		Disabled:  "#93a1a1", // base1
		Idle:      "#93a1a1", // base1
	},
	"solarized-dark": {
		Name:      "solarized-dark",
		Bg:        "#002b36", // base03
		TopBarBg:  "#073642", // base02
		Highlight: "#073642", // base02
		Text:      "#93a1a1", // base1
		Muted:     "#586e75", // base01
		Accent:    "#2aa198", // cyan
		Border:    "#586e75", // base01
		OK:        "#859900",
		Err:       "#dc322f",
		Building:  "#cb4b16",
		Pending:   "#b58900",
		Disabled:  "#586e75",
		Idle:      "#586e75",
	},
	"dark": {
		Name:      "dark",
		Bg:        "", // terminal default
		TopBarBg:  "236",
		Highlight: "237",
		Text:      "252",
		Muted:     "245",
		Accent:    "44",
		Border:    "238",
		OK:        "42",
		Err:       "203",
		Building:  "214",
		Pending:   "220",
		Disabled:  "240",
		Idle:      "244",
	},
}

// resolveTheme returns the named theme, falling back to the default.
func resolveTheme(name string) Theme {
	if t, ok := themes[name]; ok {
		return t
	}
	return themes[defaultTheme]
}

// next returns the theme after this one in the cycle order.
func (t Theme) next() Theme {
	for i, name := range themeOrder {
		if name == t.Name {
			return themes[themeOrder[(i+1)%len(themeOrder)]]
		}
	}
	return themes[defaultTheme]
}

func (t Theme) StatusColor(s tilt.Status) lipgloss.Color {
	switch s {
	case tilt.StatusOK:
		return t.OK
	case tilt.StatusError:
		return t.Err
	case tilt.StatusBuilding:
		return t.Building
	case tilt.StatusPending:
		return t.Pending
	case tilt.StatusDisabled:
		return t.Disabled
	default:
		return t.Idle
	}
}

func (t Theme) muted() lipgloss.Style  { return lipgloss.NewStyle().Foreground(t.Muted) }
func (t Theme) accent() lipgloss.Style { return lipgloss.NewStyle().Foreground(t.Accent) }
func (t Theme) err() lipgloss.Style    { return lipgloss.NewStyle().Foreground(t.Err) }
func (t Theme) warn() lipgloss.Style   { return lipgloss.NewStyle().Foreground(t.Pending) }
func (t Theme) header() lipgloss.Style { return lipgloss.NewStyle().Foreground(t.Text).Bold(true) }

// Region styles. We deliberately do NOT paint a background: filling the screen
// fights with the ANSI in Tilt's own log output and with the terminal's
// background, which renders unevenly. The palette is applied via foreground
// colors and we rely on the terminal's own background for an even surface.
func (t Theme) footer() lipgloss.Style { return lipgloss.NewStyle().Foreground(t.Muted) }
func (t Theme) pane() lipgloss.Style   { return lipgloss.NewStyle().Foreground(t.Text) }
func (t Theme) sidebar() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Text).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(t.Border)
}

// statusGlyph is theme-independent.
func statusGlyph(s tilt.Status) string {
	switch s {
	case tilt.StatusOK:
		return "✓"
	case tilt.StatusError:
		return "✕"
	case tilt.StatusBuilding:
		return "⟳"
	case tilt.StatusPending:
		return "…"
	case tilt.StatusDisabled:
		return "⊘"
	default:
		return ""
	}
}
