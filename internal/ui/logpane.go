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
	hl := lipgloss.NewStyle().Background(m.theme.Pending).Foreground(lipgloss.Color("#000000")).Bold(true)
	out := make([]string, 0, 64)
	for ln := range strings.SplitSeq(text, "\n") {
		ln = sanitizeLogLine(ln)
		if flt != "" {
			// When filtering, show the plain text with every match highlighted.
			plain := ansi.Strip(ln)
			if !strings.Contains(strings.ToLower(plain), flt) {
				continue
			}
			ln = highlightMatches(plain, m.logFilter, hl)
		}
		out = append(out, ansi.Hardwrap(ln, m.vp.Width, false))
	}
	m.vp.SetContent(strings.Join(out, "\n"))
	if m.follow {
		m.vp.GotoBottom()
	}
}

// highlightMatches wraps every case-insensitive occurrence of term in s with the
// given style, preserving the original casing of the matched text. s must be
// plain text (no ANSI). Matching is byte-aligned, which is correct for ASCII
// search terms (the common case for log search).
func highlightMatches(s, term string, style lipgloss.Style) string {
	if term == "" {
		return s
	}
	ls, lt := strings.ToLower(s), strings.ToLower(term)
	mlen := len(lt)
	var b strings.Builder
	for i := 0; i < len(s); {
		j := strings.Index(ls[i:], lt)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		j += i
		b.WriteString(s[i:j])
		b.WriteString(style.Render(s[j : j+mlen]))
		i = j + mlen
	}
	return b.String()
}

// sanitizeLogLine neutralizes terminal control characters that would corrupt the
// TUI layout. Notably curl/progress output uses carriage returns to redraw a line
// in place; rendered verbatim, a \r jumps the cursor to column 0 of the whole
// screen and overwrites the sidebar. We keep only the text after the final \r
// (what a terminal would ultimately display), turn tabs into spaces, and drop
// other C0 controls. ESC is preserved so SGR color sequences still render.
func sanitizeLogLine(s string) string {
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	if !strings.ContainsFunc(s, func(r rune) bool { return r == '\t' || (r < 0x20 && r != 0x1b) }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString("  ")
		case r == 0x1b:
			b.WriteRune(r)
		case r < 0x20:
			// drop other C0 control characters
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
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
		statusSeg := st.Label()
		if g := statusGlyph(st); g != "" {
			statusSeg = g + " " + statusSeg
		}
		if m.focus == focusLogs {
			// Focus indicator: the header takes the same reverse-video highlight the
			// sidebar selection uses, so the cursor highlight follows focus.
			plain := r.Name()
			if b := r.Backend(); b != "" {
				plain += " · " + b
			}
			if rl := r.RuntimeLine(); rl != "" {
				plain += " · " + rl
			}
			plain += " · " + statusSeg
			header = lipgloss.NewStyle().Reverse(true).Bold(true).Width(w).Render(ansi.Truncate(" "+plain, w, "…"))
		} else {
			parts := []string{m.theme.header().Render(r.Name())}
			if b := r.Backend(); b != "" {
				parts = append(parts, m.theme.accent().Render(b))
			}
			if rl := r.RuntimeLine(); rl != "" {
				parts = append(parts, m.theme.muted().Render(rl))
			}
			parts = append(parts, lipgloss.NewStyle().Foreground(m.theme.StatusColor(st)).Render(statusSeg))
			header = strings.Join(parts, m.theme.muted().Render(" · "))
		}
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
