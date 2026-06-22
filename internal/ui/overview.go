package ui

import (
	"fmt"
	"strings"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ovRow is one line of the overview: either an instance header, or one of that
// instance's non-OK resources listed beneath it. The flat slice drives both
// rendering and selection so the two never drift.
type ovRow struct {
	inst   int             // index into m.instances
	header bool            // true => instance summary row; false => resource row
	res    tilt.UIResource // the resource (valid only when !header)
}

// overviewRows builds the dashboard rows: one header per instance, followed by
// its error / building / pending resources (errors first), with healthy ones
// collapsed into the header's counts. onlyFailing drops instances with no error.
func (m Model) overviewRows() []ovRow {
	var rows []ovRow
	for i, in := range m.instances {
		v := m.views[in.Port]
		var c tilt.StatusCounts
		if v != nil {
			c = v.StatusCounts()
		}
		if m.onlyFailing && c.Error == 0 {
			continue
		}
		rows = append(rows, ovRow{inst: i, header: true})
		if v == nil {
			continue
		}
		// Errors first, then in-flight work; OK/idle/disabled are not listed.
		for _, st := range []tilt.Status{tilt.StatusError, tilt.StatusBuilding, tilt.StatusPending} {
			for _, r := range v.Resources() {
				if !r.IsDisabled() && r.State() == st {
					rows = append(rows, ovRow{inst: i, res: r})
				}
			}
		}
	}
	return rows
}

func (m *Model) clampOverview() {
	n := len(m.overviewRows())
	switch {
	case n == 0:
		m.overviewSel = 0
	case m.overviewSel >= n:
		m.overviewSel = n - 1
	case m.overviewSel < 0:
		m.overviewSel = 0
	}
}

// updateOverviewKeys handles input while the overview screen is open.
func (m Model) updateOverviewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "1", "esc":
		m.overview = false
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	case "T":
		m.theme = m.theme.next()
		return m, nil
	case "F":
		m.onlyFailing = !m.onlyFailing
		m.clampOverview()
		return m, nil
	case "up", "k":
		m.overviewSel--
		m.clampOverview()
		return m, nil
	case "down", "j":
		m.overviewSel++
		m.clampOverview()
		return m, nil
	case "enter":
		rows := m.overviewRows()
		if m.overviewSel >= 0 && m.overviewSel < len(rows) {
			return m.openOverviewRow(rows[m.overviewSel])
		}
		return m, nil
	case "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.String()[0] - '2')
		if idx < 0 || idx >= len(m.instances) {
			return m, nil // no such instance: stay in the overview
		}
		m.overview = false
		return m.gotoInstance(idx)
	}
	return m, nil
}

// openOverviewRow leaves the overview for the row's instance. For a resource row
// it also selects that resource and focuses the logs, so "spot red → ⏎ → its
// logs" is one motion.
func (m Model) openOverviewRow(r ovRow) (tea.Model, tea.Cmd) {
	m.overview = false
	var cmd tea.Cmd
	if r.inst != m.active {
		nm, c := m.gotoInstance(r.inst)
		m, cmd = nm.(Model), c
	}
	if r.header {
		m.focus = focusSidebar
	} else {
		m.focus = focusLogs
		m.selectByName(r.res.Name())
	}
	m.setLogs()
	return m, cmd
}

// selectByName points the sidebar selection at the named resource, if visible.
// Selection index 0 is the "All Resources" row, so a resource at visible()[i]
// lives at index i+1.
func (m *Model) selectByName(name string) {
	for i, r := range m.visible() {
		if r.Name() == name {
			m.selected = i + 1
			return
		}
	}
}

// instanceBadge is the compact health glyph shown after an instance's name in
// the top bar: ✕N (any error) > ⟳ (building) > … (pending) > ✓ (all ok), or a
// muted · when the instance hasn't reported yet.
func (m Model) instanceBadge(port int) (string, lipgloss.Color) {
	v := m.views[port]
	if v == nil {
		return "·", m.theme.Muted
	}
	c := v.StatusCounts()
	switch {
	case c.Error > 0:
		return fmt.Sprintf("✕%d", c.Error), m.theme.Err
	case c.Building > 0:
		return "⟳", m.theme.Building
	case c.Pending > 0:
		return "…", m.theme.Pending
	default:
		return "✓", m.theme.OK
	}
}

// renderOverview draws the cross-instance dashboard body (full width).
func (m Model) renderOverview(h int) string {
	rows := m.overviewRows()

	var lines []string
	lines = append(lines, m.overviewSummary(), "")

	if len(rows) == 0 {
		msg := "no instances"
		if m.onlyFailing {
			msg = "✓ nothing failing"
		}
		lines = append(lines, " "+m.theme.muted().Render(msg))
	}
	for i, row := range rows {
		var line string
		if row.header {
			if i > 0 {
				lines = append(lines, "") // blank line between instance blocks
			}
			line = m.renderOvHeader(row.inst, i == m.overviewSel)
		} else {
			line = m.renderOvResource(row.res, i == m.overviewSel)
		}
		lines = append(lines, ansi.Truncate(line, m.width, "…"))
	}

	body := strings.Join(lines, "\n")
	return m.theme.pane().Width(m.width).Height(h).Render(body)
}

// overviewSummary is the global rollup line across all instances.
func (m Model) overviewSummary() string {
	var agg tilt.StatusCounts
	for _, in := range m.instances {
		if v := m.views[in.Port]; v != nil {
			c := v.StatusCounts()
			agg.Error += c.Error
			agg.Building += c.Building
			agg.Pending += c.Pending
			agg.OK += c.OK
			agg.Total += c.Total
		}
	}
	segs := []string{fmt.Sprintf("%d instances", len(m.instances))}
	if agg.Error > 0 {
		segs = append(segs, lipgloss.NewStyle().Foreground(m.theme.Err).Render(fmt.Sprintf("✕%d failing", agg.Error)))
	}
	if agg.Building > 0 {
		segs = append(segs, lipgloss.NewStyle().Foreground(m.theme.Building).Render(fmt.Sprintf("⟳%d building", agg.Building)))
	}
	if agg.Pending > 0 {
		segs = append(segs, lipgloss.NewStyle().Foreground(m.theme.Pending).Render(fmt.Sprintf("…%d pending", agg.Pending)))
	}
	segs = append(segs, lipgloss.NewStyle().Foreground(m.theme.OK).Render(fmt.Sprintf("✓%d / %d ok", agg.OK, agg.Total)))
	if m.onlyFailing {
		segs = append(segs, m.theme.accent().Render("[only-failing]"))
	}
	title := m.theme.accent().Bold(true).Render("OVERVIEW")
	return " " + title + "   " + strings.Join(segs, m.theme.muted().Render(" · "))
}

// Overview column widths (display cells). Header and resource rows share these
// offsets so the failing-resource sub-rows line up under their instance, with
// generous gutters between columns.
const (
	ovTagW   = 6  // "‹2›" plus breathing room
	ovNameW  = 26 // instance label / resource name
	ovPortW  = 16 // ":10350"
	ovBadgeW = 18 // ✕N ⟳N …N
)

func (m Model) renderOvHeader(i int, sel bool) string {
	in := m.instances[i]
	var c tilt.StatusCounts
	if v := m.views[in.Port]; v != nil {
		c = v.StatusCounts()
	}
	// Instances are numbered ‹2›, ‹3›, … to match the top bar and the digit keys
	// that jump to them (‹1› is the overview itself).
	tag := m.theme.accent().Render(fmt.Sprintf("‹%d›", i+2))
	name := m.theme.header().Render(in.Label)
	port := m.theme.muted().Render(fmt.Sprintf(":%d", in.Port))
	ok := lipgloss.NewStyle().Foreground(m.theme.OK).Render(fmt.Sprintf("✓%d/%d", c.OK, c.Total))
	return ovMarker(m.theme, sel) +
		cell(tag, ovTagW) + cell(name, ovNameW) + cell(port, ovPortW) +
		cell(m.ovBadges(in.Port, c), ovBadgeW) + ok
}

func (m Model) renderOvResource(r tilt.UIResource, sel bool) string {
	st := r.State()
	glyph := lipgloss.NewStyle().Foreground(m.theme.StatusColor(st)).Render(statusGlyph(st))
	name := lipgloss.NewStyle().Foreground(m.theme.Text).Render(r.Name())
	detail := r.Backend()
	if rl := r.RuntimeLine(); rl != "" {
		if detail != "" {
			detail += " · "
		}
		detail += rl
	}
	if detail == "" {
		detail = st.Label()
	}
	// Indent so the resource name lines up under the instance-name column (its
	// glyph sits just to the left); the detail then aligns under the port column.
	// ovTagW-2 keeps the name under the header's name regardless of the tag width.
	prefix := ovMarker(m.theme, sel) + strings.Repeat(" ", ovTagW-2) + glyph + " "
	return prefix + cell(name, ovNameW) + m.theme.muted().Render(detail)
}

// ovBadges renders an instance's error/building/pending tallies, or "healthy" /
// a muted "·" (not yet reported) when there's nothing to flag.
func (m Model) ovBadges(port int, c tilt.StatusCounts) string {
	if m.views[port] == nil {
		return m.theme.muted().Render("·")
	}
	var segs []string
	if c.Error > 0 {
		segs = append(segs, lipgloss.NewStyle().Foreground(m.theme.Err).Render(fmt.Sprintf("✕%d", c.Error)))
	}
	if c.Building > 0 {
		segs = append(segs, lipgloss.NewStyle().Foreground(m.theme.Building).Render(fmt.Sprintf("⟳%d", c.Building)))
	}
	if c.Pending > 0 {
		segs = append(segs, lipgloss.NewStyle().Foreground(m.theme.Pending).Render(fmt.Sprintf("…%d", c.Pending)))
	}
	if len(segs) == 0 {
		return m.theme.muted().Render("healthy")
	}
	return strings.Join(segs, " ")
}

// ovMarker is the 2-cell selection gutter shared by header and resource rows, so
// the ▶ always sits in the same left column.
func ovMarker(th Theme, sel bool) string {
	if sel {
		return th.accent().Render("▶") + " "
	}
	return "  "
}

// cell truncates a styled (possibly ANSI) string to w display cells and right-
// pads it back to w, so columns line up regardless of content length.
func cell(s string, w int) string {
	return pad2(ansi.Truncate(s, w, "…"), w)
}

// pad2 right-pads an already-styled (possibly ANSI) string to w cells.
func pad2(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
