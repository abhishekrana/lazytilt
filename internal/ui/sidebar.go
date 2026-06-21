package ui

import (
	"strings"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const sidebarWidth = 28

// visible returns the resources shown in the sidebar: sorted by Tilt's order,
// with disabled ones hidden unless toggled, filtered by the name filter.
func (m Model) visible() []tilt.UIResource {
	if m.view == nil {
		return nil
	}
	flt := strings.ToLower(m.resFilter)
	var out []tilt.UIResource
	for _, r := range m.view.Resources() {
		if r.IsDisabled() && !m.showDisabled {
			continue
		}
		if flt != "" && !strings.Contains(strings.ToLower(r.Name()), flt) {
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
		block := lipgloss.NewStyle().Foreground(m.theme.StatusColor(st)).Render("▌")

		nameStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
		if st == tilt.StatusDisabled {
			nameStyle = m.theme.muted()
		}
		ind := " "
		if i == m.selected {
			ind = m.theme.accent().Render("▶")
			nameStyle = nameStyle.Bold(true)
		}
		row := ind + block + " " + nameStyle.Render(r.Name())
		if st == tilt.StatusError {
			row += " " + lipgloss.NewStyle().Foreground(m.theme.Err).Render("✕")
		}
		row = ansi.Truncate(row, sidebarWidth, "…")
		if i == m.selected {
			row = lipgloss.NewStyle().Background(m.theme.Highlight).Width(sidebarWidth).Render(row)
		}
		lines = append(lines, row)
	}
	if len(vis) == 0 {
		lines = append(lines, m.theme.muted().Render(" no resources"))
	}
	body := strings.Join(lines, "\n")
	return m.theme.sidebar().Width(sidebarWidth).Height(h).Render(body)
}
