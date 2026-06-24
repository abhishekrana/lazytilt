package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const sidebarWidth = 28

// noLabelHeader is the group header for resources that carry no label.
const noLabelHeader = "(no label)"

// The sidebar's selectable rows are a synthetic "All Resources" entry (index 0,
// whose logs are the combined stream of every resource) followed by the
// resources (index i maps to visible()[i-1]). When any resource has a label the
// resources are shown in label groups with non-selectable header rows between
// them; selection still moves resource-to-resource, skipping the headers.

// sidebarGroup is a label group rendered in the sidebar: a header (empty for the
// unlabeled flat fallback) and its resources in display order.
type sidebarGroup struct {
	header    string
	resources []tilt.UIResource
}

// sidebarGroups arranges the resources for display. If any resource has a label,
// resources are grouped by their first label — groups sorted by label name with
// "(no label)" last, resources alphabetical within each group. With no labels
// anywhere it falls back to a single headerless group in Tilt's order.
func (m Model) sidebarGroups() []sidebarGroup {
	if m.view == nil {
		return nil
	}
	all := m.view.Resources()

	labeled := false
	for i := range all {
		if len(all[i].LabelNames()) > 0 {
			labeled = true
			break
		}
	}
	if !labeled {
		return []sidebarGroup{{resources: all}} // flat fallback, Tilt order, no header
	}

	byLabel := map[string][]tilt.UIResource{}
	for _, r := range all {
		key := noLabelHeader
		if names := r.LabelNames(); len(names) > 0 {
			key = names[0] // first label (LabelNames is sorted)
		}
		byLabel[key] = append(byLabel[key], r)
	}

	headers := make([]string, 0, len(byLabel))
	for k := range byLabel {
		headers = append(headers, k)
	}
	sort.Slice(headers, func(i, j int) bool {
		if headers[i] == noLabelHeader {
			return false
		}
		if headers[j] == noLabelHeader {
			return true
		}
		return headers[i] < headers[j]
	})

	groups := make([]sidebarGroup, 0, len(headers))
	for _, h := range headers {
		rs := byLabel[h]
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].Name() < rs[j].Name() })
		groups = append(groups, sidebarGroup{header: h, resources: rs})
	}
	return groups
}

// visible returns the resources in sidebar display order (grouped + alphabetical
// when labeled, else Tilt order). Disabled resources are always included.
func (m Model) visible() []tilt.UIResource {
	var out []tilt.UIResource
	for _, g := range m.sidebarGroups() {
		out = append(out, g.resources...)
	}
	return out
}

// rowCount is the number of selectable sidebar rows: the All-Resources row plus
// one per resource (0 before any view has loaded). Group headers are not counted
// — they are not selectable.
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
	m.log.follow = true // new selection: show its newest logs
	m.setLogs()
}

func (m Model) renderSidebar(h int) string {
	lines := make([]string, 0, h)
	if m.view != nil {
		lines = append(lines, m.allRow())
	}

	idx := 0 // running index into visible(); selection index is idx+1
	for _, g := range m.sidebarGroups() {
		if g.header != "" {
			lines = append(lines, "", m.groupHeader(g))
		}
		for _, r := range g.resources {
			lines = append(lines, m.resourceRow(idx, r))
			idx++
		}
	}
	if m.view != nil && idx == 0 {
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

// groupHeader renders a non-selectable label divider with a rolled-up status.
func (m Model) groupHeader(g sidebarGroup) string {
	row := m.theme.accent().Bold(true).Render(g.header)
	if roll := m.groupRollup(g.resources); roll != "" {
		row += " " + roll
	}
	return ansi.Truncate(row, sidebarWidth, "…")
}

// groupRollup summarizes a group's resources as colored badges: errors, then
// in-flight work, disabled, and finally the healthy count.
func (m Model) groupRollup(rs []tilt.UIResource) string {
	var errs, building, pending, ok, disabled int
	for i := range rs {
		switch rs[i].State() {
		case tilt.StatusError:
			errs++
		case tilt.StatusBuilding:
			building++
		case tilt.StatusPending:
			pending++
		case tilt.StatusOK:
			ok++
		case tilt.StatusDisabled:
			disabled++
		}
	}
	badge := func(c lipgloss.Color, glyph string, n int) string {
		return lipgloss.NewStyle().Foreground(c).Render(fmt.Sprintf("%s%d", glyph, n))
	}
	var segs []string
	if errs > 0 {
		segs = append(segs, badge(m.theme.Err, "✕", errs))
	}
	if building > 0 {
		segs = append(segs, badge(m.theme.Building, "⟳", building))
	}
	if pending > 0 {
		segs = append(segs, badge(m.theme.Pending, "…", pending))
	}
	if disabled > 0 {
		segs = append(segs, badge(m.theme.Disabled, "⊘", disabled))
	}
	if ok > 0 {
		segs = append(segs, badge(m.theme.OK, "✓", ok))
	}
	return strings.Join(segs, " ")
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
