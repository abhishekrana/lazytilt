package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// logLevel is the log-pane level filter.
type logLevel int

const (
	levelAll logLevel = iota
	levelError
	levelWarn
)

func (l logLevel) allows(level string) bool {
	switch l {
	case levelError:
		return level == "ERROR"
	case levelWarn:
		return level == "WARN" || level == "ERROR"
	default:
		return true
	}
}

func (l logLevel) label() string {
	switch l {
	case levelError:
		return "errors"
	case levelWarn:
		return "warnings"
	default:
		return "all"
	}
}

// setLogs rebuilds the log viewport content for the selected resource, applying
// the level and text filters and wrapping to the pane width.
func (m *Model) setLogs() {
	if m.vp.Width <= 0 {
		return
	}
	r, ok := m.selectedResource()
	if !ok || m.view == nil {
		m.vp.SetContent("")
		return
	}

	var b strings.Builder
	for _, s := range m.view.LogList.SegmentsFor(r.Name()) {
		if m.level.allows(s.Level) {
			b.WriteString(s.Text)
		}
	}
	text := strings.TrimRight(b.String(), "\n")

	flt := strings.ToLower(m.logFilter)
	out := make([]string, 0, 64)
	for ln := range strings.SplitSeq(text, "\n") {
		if flt != "" && !strings.Contains(strings.ToLower(ansi.Strip(ln)), flt) {
			continue
		}
		out = append(out, ansi.Hardwrap(ln, m.vp.Width, false))
	}
	m.vp.SetContent(strings.Join(out, "\n"))
	if m.follow {
		m.vp.GotoBottom()
	}
}

func (m Model) renderRightPane(w, h int) string {
	box := m.theme.pane().Width(w).Height(h)

	if m.view == nil {
		msg := "connecting…"
		if m.loadErr != nil {
			msg = "error: " + m.loadErr.Error()
		}
		return box.Render(m.theme.muted().Render(msg))
	}

	r, ok := m.selectedResource()
	header := m.theme.muted().Render("no resource selected")
	if ok {
		st := r.State()
		parts := []string{m.theme.header().Render(r.Name())}
		if b := r.Backend(); b != "" {
			parts = append(parts, m.theme.accent().Render(b))
		}
		if rl := r.RuntimeLine(); rl != "" {
			parts = append(parts, m.theme.muted().Render(rl))
		}
		statusSeg := st.Label()
		if g := statusGlyph(st); g != "" {
			statusSeg = g + " " + statusSeg
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(m.theme.StatusColor(st)).Render(statusSeg))
		header = strings.Join(parts, m.theme.muted().Render(" · "))
	}

	follow := "follow off"
	if m.follow {
		follow = "follow on"
	}
	controls := m.theme.muted().Render("level ") + m.level.label()
	if m.logFilter != "" {
		controls += m.theme.muted().Render("  filter ") + m.logFilter
	}
	controls += m.theme.muted().Render("  [" + follow + "]")

	sep := lipgloss.NewStyle().Foreground(m.theme.Border).Render(strings.Repeat("─", w))

	body := lipgloss.JoinVertical(lipgloss.Left,
		ansi.Truncate(header, w, "…"),
		ansi.Truncate(controls, w, "…"),
		sep,
		m.vp.View(),
	)
	return box.Render(body)
}
