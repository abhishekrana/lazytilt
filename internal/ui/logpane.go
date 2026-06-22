package ui

import (
	"fmt"
	"strings"

	"github.com/abhishekrana/lazytilt/internal/tilt"
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

// setLogs rebuilds the log viewport content for the current selection — either a
// single resource or the combined "All Resources" stream — applying the level and
// text filters and wrapping to the pane width.
func (m *Model) setLogs() {
	if m.vp.Width <= 0 {
		return
	}
	r, ok := m.selectedResource()

	// Size the viewport around the always-on detail strip: header + controls +
	// separator take 3 rows, plus one row per detail line for the selection. The
	// combined "All Resources" view has no detail strip.
	bodyH := max(m.height-topBarHeight-footerHeight, 3)
	strip := 0
	if ok {
		strip = len(m.detailLines(r, m.vp.Width))
	}
	m.vp.Height = max(bodyH-3-strip, 1)

	if m.view == nil || (!ok && !m.onAllLogs()) {
		m.vp.SetContent("")
		return
	}

	// Source lines: the combined stream (All Resources) or the selected resource.
	var lines []string
	if m.onAllLogs() {
		lines = m.combinedLogLines()
	} else {
		lines = m.resourceLogLines(r)
	}

	flt := strings.ToLower(m.logFilter)
	hl := lipgloss.NewStyle().Background(m.theme.Pending).Foreground(lipgloss.Color("#000000")).Bold(true)
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
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

// resourceLogLines returns the selected resource's logs as sanitized lines,
// honoring the level filter.
func (m Model) resourceLogLines(r tilt.UIResource) []string {
	var b strings.Builder
	for _, s := range m.view.LogList.SegmentsFor(r.Name()) {
		if m.level.allows(s.Level) {
			b.WriteString(s.Text)
		}
	}
	text := strings.TrimRight(b.String(), "\n")
	if text == "" {
		return nil
	}
	out := make([]string, 0, 64)
	for ln := range strings.SplitSeq(text, "\n") {
		out = append(out, sanitizeLogLine(ln))
	}
	return out
}

// maxSourceWidth caps the resource-name column in the combined view.
const maxSourceWidth = 18

// combinedLogLines returns the interleaved logs of every resource as sanitized
// lines, each prefixed with its source ("tilt" = global Tilt output) in the
// source's status color, honoring the level filter. The source column is
// width-aligned so the log text starts at a consistent column.
func (m Model) combinedLogLines() []string {
	all := m.view.LogList.AllLines()

	// Status color per resource, for the source prefix.
	color := map[string]lipgloss.Color{}
	for _, r := range m.view.Resources() {
		color[r.Name()] = m.theme.StatusColor(r.State())
	}

	w := 0
	for _, ll := range all {
		if !m.level.allows(ll.Level) {
			continue
		}
		if n := lipgloss.Width(sourceName(ll.Manifest)); n > w {
			w = n
		}
	}
	if w > maxSourceWidth {
		w = maxSourceWidth
	}

	out := make([]string, 0, len(all))
	for _, ll := range all {
		if !m.level.allows(ll.Level) {
			continue
		}
		name := ansi.Truncate(sourceName(ll.Manifest), w, "…")
		if pad := w - lipgloss.Width(name); pad > 0 {
			name += strings.Repeat(" ", pad)
		}
		style := m.theme.muted()
		if c, ok := color[ll.Manifest]; ok {
			style = lipgloss.NewStyle().Foreground(c)
		}
		prefix := style.Render(name) + m.theme.muted().Render(" │ ")
		out = append(out, prefix+sanitizeLogLine(ll.Text))
	}
	return out
}

// sourceName labels a log line's manifest in the combined view; global
// (empty-manifest) Tilt output is labeled "tilt".
func sourceName(manifest string) string {
	if manifest == "" {
		return "tilt"
	}
	return manifest
}

// resourceLogText assembles the full, plain-text logs for a resource: every
// segment, every level, with carriage-return/control noise neutralized and ANSI
// color stripped. It backs the "save logs" action, so the file opens cleanly in
// an editor rather than carrying escape codes.
func (m Model) resourceLogText(r tilt.UIResource) string {
	if m.view == nil {
		return ""
	}
	var b strings.Builder
	for _, s := range m.view.LogList.SegmentsFor(r.Name()) {
		b.WriteString(s.Text)
	}
	out := make([]string, 0, 64)
	for ln := range strings.SplitSeq(strings.TrimRight(b.String(), "\n"), "\n") {
		out = append(out, ansi.Strip(sanitizeLogLine(ln)))
	}
	return strings.Join(out, "\n") + "\n"
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

// allLogsHeader is the log-pane header for the combined "All Resources" view.
func (m Model) allLogsHeader(w int) string {
	n := m.view.StatusCounts().Total
	if m.focus == focusLogs {
		// Focus indicator: same reverse-video highlight a resource header uses.
		plain := fmt.Sprintf("All Resources · %d resources", n)
		return lipgloss.NewStyle().Reverse(true).Bold(true).Width(w).Render(ansi.Truncate(" "+plain, w, "…"))
	}
	return m.theme.header().Render("All Resources") + m.theme.muted().Render(fmt.Sprintf(" · %d resources", n))
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
	switch {
	case m.onAllLogs():
		header = m.allLogsHeader(w)
	case ok:
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

	rows := []string{ansi.Truncate(header, w, "…")}
	if ok {
		rows = append(rows, m.detailLines(r, w)...)
	}
	rows = append(rows, ansi.Truncate(controls, w, "…"), sep, m.vp.View())
	return box.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
