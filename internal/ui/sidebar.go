package ui

import (
	"strings"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const sidebarWidth = 28

// visible returns the resources shown in the sidebar: sorted by Tilt's order,
// with disabled ones hidden unless toggled.
func (m Model) visible() []tilt.UIResource {
	if m.view == nil {
		return nil
	}
	var out []tilt.UIResource
	for _, r := range m.view.Resources() {
		if r.IsDisabled() && !m.showDisabled {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (m Model) selectedResource() (tilt.UIResource, bool) {
	vis := m.visible()
	if m.selected < 0 || m.selected >= len(vis) {
		return tilt.UIResource{}, false
	}
	return vis[m.selected], true
}

func (m *Model) clampSelection() {
	n := len(m.visible())
	switch {
	case n == 0:
		m.selected = 0
	case m.selected >= n:
		m.selected = n - 1
	case m.selected < 0:
		m.selected = 0
	}
}

func (m *Model) moveSelection(d int) {
	m.selected += d
	m.clampSelection()
	m.setLogs()
}

func (m Model) renderSidebar(h int) string {
	vis := m.visible()
	lines := make([]string, 0, h)
	for i, r := range vis {
		st := r.State()
		g := statusGlyph(st)

		if i == m.selected {
			// Selected row: a single reverse-video bar spanning the full width.
			// Built as plain text (no inline ANSI) so the highlight is uniform.
			text := "▶ "
			if g != "" {
				text += g + " "
			}
			text += r.Name()
			text = ansi.Truncate(text, sidebarWidth, "…")
			lines = append(lines, lipgloss.NewStyle().Reverse(true).Bold(true).Width(sidebarWidth).Render(text))
			continue
		}

		block := lipgloss.NewStyle().Foreground(m.theme.StatusColor(st)).Render("▌")
		nameStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
		if st == tilt.StatusDisabled {
			nameStyle = m.theme.muted()
		}
		row := " " + block + " " + nameStyle.Render(r.Name())
		if st == tilt.StatusError {
			row += " " + lipgloss.NewStyle().Foreground(m.theme.Err).Render("✕")
		}
		lines = append(lines, ansi.Truncate(row, sidebarWidth, "…"))
	}
	if len(vis) == 0 {
		lines = append(lines, m.theme.muted().Render(" no resources"))
	}
	body := strings.Join(lines, "\n")
	return m.theme.sidebar().Width(sidebarWidth).Height(h).Render(body)
}
