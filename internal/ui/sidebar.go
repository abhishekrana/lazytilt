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

// sidebarRow is one selectable sidebar entry: a resource, or — when workload is
// non-empty — one workload (deployment/statefulset) nested under that resource.
// The synthetic "All Resources" row (selection index 0) is not represented here.
type sidebarRow struct {
	resource tilt.UIResource
	workload string // "" = the resource itself; else a child workload
}

// selectableRows is the flat, ordered list of navigable sidebar entries below the
// All-Resources row: each resource followed by its workloads (only when it bundles
// more than one). Selection index i+1 maps to selectableRows()[i]; this must stay
// in lockstep with renderSidebar's row order.
func (m Model) selectableRows() []sidebarRow {
	var out []sidebarRow
	for _, g := range m.sidebarGroups() {
		for _, r := range g.resources {
			out = append(out, sidebarRow{resource: r})
			if wl := r.Workloads(); len(wl) > 1 {
				for _, w := range wl {
					out = append(out, sidebarRow{resource: r, workload: w})
				}
			}
		}
	}
	return out
}

// visible returns the resources in sidebar display order (grouped + alphabetical
// when labeled, else Tilt order), without the workload child rows. Selection
// indexing uses selectableRows; this is for plain resource-list needs.
func (m Model) visible() []tilt.UIResource {
	var out []tilt.UIResource
	for _, g := range m.sidebarGroups() {
		out = append(out, g.resources...)
	}
	return out
}

// rowCount is the number of selectable sidebar rows: the All-Resources row plus
// every resource and workload row (0 before any view has loaded). Group headers
// are not counted — they are not selectable.
func (m Model) rowCount() int {
	if m.view == nil {
		return 0
	}
	return len(m.selectableRows()) + 1
}

// onAllLogs reports whether the "All Resources" row (index 0) is selected.
func (m Model) onAllLogs() bool { return m.view != nil && m.selected == 0 }

// selectedRow returns the selected sidebar row (resource or workload), or
// ok=false on the "All Resources" row (or nothing).
func (m Model) selectedRow() (sidebarRow, bool) {
	rows := m.selectableRows()
	i := m.selected - 1
	if i < 0 || i >= len(rows) {
		return sidebarRow{}, false
	}
	return rows[i], true
}

// selectedResource returns the resource owning the selected row — the parent
// release when a workload child is selected — or ok=false on "All Resources".
// Resource actions (trigger/enable/disable) therefore target the parent, which is
// the only granularity the tilt CLI offers.
func (m Model) selectedResource() (tilt.UIResource, bool) {
	row, ok := m.selectedRow()
	if !ok {
		return tilt.UIResource{}, false
	}
	return row.resource, true
}

// selectedWorkload returns the (manifest, workload) of the selected workload
// child row, or ok=false when a plain resource or "All Resources" is selected.
func (m Model) selectedWorkload() (manifest, workload string, ok bool) {
	row, rok := m.selectedRow()
	if !rok || row.workload == "" {
		return "", "", false
	}
	return row.resource.Name(), row.workload, true
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

	sel := 0 // running selection index; 0 (the All row) was emitted above
	for _, g := range m.sidebarGroups() {
		if g.header != "" {
			lines = append(lines, "", m.groupHeader(g))
		}
		for _, r := range g.resources {
			sel++
			lines = append(lines, m.resourceRow(sel, r))
			// A helm_resource bundles a whole release under one row; show its inner
			// workloads as selectable children (their logs filter to that workload's
			// pods). Single-workload resources stay flat — the child would just echo
			// the row. ponytail: rows past the sidebar height are clipped by the box —
			// fine until a release has dozens of workloads.
			if wl := r.Workloads(); len(wl) > 1 {
				for _, w := range wl {
					sel++
					lines = append(lines, m.workloadRow(sel, w))
				}
			}
		}
	}
	if m.view != nil && sel == 0 {
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

// workloadRow renders a selectable child row under a k8s resource — one inner
// workload (deployment/statefulset) of a bundled helm release. Selecting it
// filters the log pane to that workload's pods. sel is its selection index.
func (m Model) workloadRow(sel int, name string) string {
	if sel == m.selected && m.focus == focusSidebar {
		text := ansi.Truncate("▶ └ "+name, sidebarWidth, "…")
		return lipgloss.NewStyle().Reverse(true).Bold(true).Width(sidebarWidth).Render(text)
	}
	ind := " "
	if sel == m.selected {
		ind = m.theme.accent().Render("▶")
	}
	row := ind + " " + m.theme.muted().Render("└ "+name)
	return ansi.Truncate(row, sidebarWidth, "…")
}

// resourceRow renders a resource at the given selection index.
func (m Model) resourceRow(sel int, r tilt.UIResource) string {
	st := r.State()

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
