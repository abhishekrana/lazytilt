package ui

import (
	"strings"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const sidebarWidth = 28

// The sidebar's selectable rows are a synthetic "All Resources" entry (index 0,
// whose logs are the combined stream of every resource) followed by the
// resources (index i maps to visible()[i-1]).

// visible returns the resources shown in the sidebar, sorted by Tilt's order.
// Disabled resources are always shown (so you can select one and re-enable it).
func (m Model) visible() []tilt.UIResource {
	if m.view == nil {
		return nil
	}
	return m.view.Resources()
}

// rowCount is the number of selectable sidebar rows: the All-Resources row plus
// one per resource (0 before any view has loaded).
func (m Model) rowCount() int {
	if m.view == nil {
		return 0
	}
	return len(m.visible()) + 1
}

// onAllLogs reports whether the "All Resources" row (index 0) is selected.
func (m Model) onAllLogs() bool { return m.view != nil && m.selected == 0 }

// selectedResource returns the selected resource, or ok=false when the
// "All Resources" row (or nothing) is selected.
func (m Model) selectedResource() (tilt.UIResource, bool) {
	vis := m.visible()
	i := m.selected - 1
	if i < 0 || i >= len(vis) {
		return tilt.UIResource{}, false
	}
	return vis[i], true
}

func (m *Model) clampSelection() {
	n := m.rowCount()
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

	if m.view != nil {
		lines = append(lines, m.allRow())
	}
	for i, r := range vis {
		lines = append(lines, m.resourceRow(i, r))
	}
	if m.view != nil && len(vis) == 0 {
		lines = append(lines, m.theme.muted().Render(" no resources"))
	}

	body := strings.Join(lines, "\n")
	return m.theme.sidebar().Width(sidebarWidth).Height(h).Render(body)
}

// allRow renders the synthetic "All Resources" entry (selection index 0).
func (m Model) allRow() string {
	const label = "All Resources"
	if m.selected == 0 && m.focus == focusSidebar {
		text := ansi.Truncate("▶ ≡ "+label, sidebarWidth, "…")
		return lipgloss.NewStyle().Reverse(true).Bold(true).Width(sidebarWidth).Render(text)
	}
	ind := " "
	if m.selected == 0 {
		ind = m.theme.accent().Render("▶")
	}
	row := ind + m.theme.muted().Render("≡") + " " + lipgloss.NewStyle().Foreground(m.theme.Text).Render(label)
	return ansi.Truncate(row, sidebarWidth, "…")
}

// resourceRow renders the resource at visible()[i], i.e. selection index i+1.
func (m Model) resourceRow(i int, r tilt.UIResource) string {
	st := r.State()
	sel := i + 1

	// The selected row gets the bright reverse-video bar only while the sidebar
	// is focused. When focus moves to the logs, the highlight moves with it (to
	// the log header) and the sidebar row reverts to normal, keeping the ▶ arrow.
	if sel == m.selected && m.focus == focusSidebar {
		g := statusGlyph(st)
		text := "▶ "
		if g != "" {
			text += g + " "
		}
		text += r.Name()
		text = ansi.Truncate(text, sidebarWidth, "…")
		return lipgloss.NewStyle().Reverse(true).Bold(true).Width(sidebarWidth).Render(text)
	}

	block := lipgloss.NewStyle().Foreground(m.theme.StatusColor(st)).Render("▌")
	nameStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	if st == tilt.StatusDisabled {
		nameStyle = m.theme.muted()
	}
	ind := " "
	if sel == m.selected {
		ind = m.theme.accent().Render("▶")
	}
	row := ind + block + " " + nameStyle.Render(r.Name())
	if st == tilt.StatusError {
		row += " " + lipgloss.NewStyle().Foreground(m.theme.Err).Render("✕")
	}
	return ansi.Truncate(row, sidebarWidth, "…")
}
