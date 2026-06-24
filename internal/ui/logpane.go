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
	// The log pane is hidden behind the overview screen, so there's nothing to do.
	if m.overview {
		return
	}
	if m.width == 0 || m.height == 0 {
		return // no terminal size yet
	}
	w := max(m.width-sidebarWidth-1, 10)
	r, ok := m.selectedResource()

	// Size the log body around the always-on detail strip: header + controls +
	// separator take 3 rows, plus one row per detail line for the selection. The
	// combined "All Resources" view has no detail strip.
	bodyH := max(m.height-topBarHeight-footerHeight, 3)
	strip := 0
	if ok {
		strip = len(m.detailLines(r, w))
	}
	m.log.resize(w, max(bodyH-3-strip, 1))

	if m.view == nil || (!ok && !m.onAllLogs()) {
		m.log.setLines(nil)
		return
	}

	// Assemble the logical lines (combined stream or single resource), apply the
	// text filter, and hand them to the windowed renderer — which wraps and paints
	// only the visible rows. Assembly is O(buffer) but bounded by the segment cap;
	// the per-frame render is O(visible).
	if m.onAllLogs() {
		m.log.setLines(m.filterLines(m.combinedLogLines()))
	} else {
		m.log.setLines(m.filterLines(m.resourceLogLines(r)))
	}
}

// filterLines applies the text filter (m.logFilter): it keeps only lines that
// contain the term (case-insensitive) and highlights every match. With no filter
// it returns the lines unchanged. Level filtering already happened in assembly.
func (m Model) filterLines(lines []string) []string {
	if m.logFilter == "" {
		return lines
	}
	flt := strings.ToLower(m.logFilter)
	hl := lipgloss.NewStyle().Background(m.theme.Pending).Foreground(lipgloss.Color("#000000")).Bold(true)
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		plain := ansi.Strip(ln)
		if !strings.Contains(strings.ToLower(plain), flt) {
			continue
		}
		out = append(out, highlightMatches(plain, m.logFilter, hl))
	}
	return out
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

	// Status color per resource (for the prefix) and the source-name column width,
	// both sized once from the resource set plus the global "tilt" label. Deriving
	// the width from the resource set (not by scanning every log line) is O(res)
	// and stable — a new source's first line never shifts the alignment of prior
	// lines.
	color := map[string]lipgloss.Color{}
	w := lipgloss.Width("tilt")
	for _, r := range m.view.Resources() {
		color[r.Name()] = m.theme.StatusColor(r.State())
		if n := lipgloss.Width(r.Name()); n > w {
			w = n
		}
	}
	if w > maxSourceWidth {
		w = maxSourceWidth
	}

	// Build each source's styled prefix once (there are few sources), then just
	// concatenate per line — styling every line with lipgloss was the dominant
	// per-frame cost on a busy combined stream.
	sep := m.theme.muted().Render(" │ ")
	prefixes := map[string]string{}
	prefixFor := func(manifest string) string {
		if p, ok := prefixes[manifest]; ok {
			return p
		}
		name := ansi.Truncate(sourceName(manifest), w, "…")
		if pad := w - lipgloss.Width(name); pad > 0 {
			name += strings.Repeat(" ", pad)
		}
		style := m.theme.muted()
		if c, ok := color[manifest]; ok {
			style = lipgloss.NewStyle().Foreground(c)
		}
		p := style.Render(name) + sep
		prefixes[manifest] = p
		return p
	}

	out := make([]string, 0, len(all))
	for _, ll := range all {
		if !m.level.allows(ll.Level) {
			continue
		}
		out = append(out, prefixFor(ll.Manifest)+sanitizeLogLine(ll.Text))
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
	// Drop trailing CR first so CRLF ("…\r\n", split into "…\r") line endings keep
	// their text. Only an *interior* CR is a progress/overwrite sequence where the
	// final write wins — collapse those to the text after the last CR.
	s = strings.TrimRight(s, "\r")
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
	if m.log.follow {
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
	rows = append(rows, ansi.Truncate(controls, w, "…"), sep, m.log.View())
	return box.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
