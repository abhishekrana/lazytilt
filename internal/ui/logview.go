package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// logView renders a scrollable window over already-decorated logical log lines,
// wrapping and painting ONLY the visible rows. Cost is O(visible) per frame
// regardless of buffer size — the lazytilt analogue of how Tilt's web UI windows
// its log rendering. Lines arrive decorated (source prefix, highlight, sanitized)
// but NOT wrapped; wrapping to the current width happens here.
type logView struct {
	lines  []string
	width  int // content columns to wrap to
	height int // visible rows
	top    pos // first visible visual row
	follow bool
}

// pos is a scroll position: a logical line index plus the visual row within that
// line's wrapped form. Tracking the position this way keeps scrolling O(distance
// moved), never requiring a visual-row count of the whole buffer.
type pos struct {
	logical int
	row     int
}

// wrapRows returns the visual rows a logical line occupies at the current width.
func (v *logView) wrapRows(i int) []string {
	if v.width < 1 || i < 0 || i >= len(v.lines) {
		return nil
	}
	return strings.Split(ansi.Hardwrap(v.lines[i], v.width, false), "\n")
}

// rowCount is how many visual rows logical line i wraps to (at least 1).
func (v *logView) rowCount(i int) int {
	if n := len(v.wrapRows(i)); n > 0 {
		return n
	}
	return 1
}

func (v *logView) setLines(lines []string) {
	v.lines = lines
	v.clamp()
}

func (v *logView) resize(width, height int) {
	v.width = max(width, 1)
	v.height = max(height, 1)
	v.clamp()
}

func (v *logView) gotoTop() {
	v.follow = false
	v.top = pos{}
}

func (v *logView) scrollUp(n int) {
	v.follow = false
	for ; n > 0; n-- {
		v.top = v.prevRow(v.top)
	}
}

func (v *logView) scrollDown(n int) {
	bt := v.bottomTop()
	for ; n > 0 && v.before(v.top, bt); n-- {
		v.top = v.nextRow(v.top)
	}
	if v.after(v.top, bt) {
		v.top = bt
	}
}

// before/after compare scroll positions.
func (v *logView) before(a, b pos) bool {
	return a.logical < b.logical || (a.logical == b.logical && a.row < b.row)
}
func (v *logView) after(a, b pos) bool { return v.before(b, a) }

// nextRow / prevRow step one visual row, crossing logical-line boundaries and
// clamping at the ends.
func (v *logView) nextRow(p pos) pos {
	if p.row+1 < v.rowCount(p.logical) {
		return pos{p.logical, p.row + 1}
	}
	if p.logical+1 < len(v.lines) {
		return pos{p.logical + 1, 0}
	}
	return p
}

func (v *logView) prevRow(p pos) pos {
	if p.row > 0 {
		return pos{p.logical, p.row - 1}
	}
	if p.logical > 0 {
		return pos{p.logical - 1, v.rowCount(p.logical-1) - 1}
	}
	return pos{}
}

// bottomTop is the top position of the last full page — walk backward from the
// final line accumulating visual rows until the window is filled. Handles a
// single line taller than the window (shows its tail).
func (v *logView) bottomTop() pos {
	need := v.height
	for i := len(v.lines) - 1; i >= 0; i-- {
		rc := v.rowCount(i)
		if rc >= need {
			return pos{i, rc - need}
		}
		need -= rc
	}
	return pos{}
}

// clamp keeps top valid: pinned to the bottom when following, otherwise within
// bounds (and snapped to the bottom if the buffer rotated past the old top).
func (v *logView) clamp() {
	if len(v.lines) == 0 {
		v.top = pos{}
		return
	}
	if v.follow || v.top.logical >= len(v.lines) {
		v.top = v.bottomTop()
		return
	}
	if rc := v.rowCount(v.top.logical); v.top.row >= rc {
		v.top.row = rc - 1
	}
	if bt := v.bottomTop(); v.after(v.top, bt) {
		v.top = bt
	}
}

// View renders exactly height rows (padded), each wrapped/padded to width.
func (v *logView) View() string {
	rows := make([]string, 0, v.height)
	for i := v.top.logical; i < len(v.lines) && len(rows) < v.height; i++ {
		wr := v.wrapRows(i)
		start := 0
		if i == v.top.logical {
			start = v.top.row
		}
		for r := start; r < len(wr) && len(rows) < v.height; r++ {
			rows = append(rows, wr[r])
		}
	}
	for len(rows) < v.height {
		rows = append(rows, "")
	}
	// Pad/truncate each row to width (ANSI-aware), so the bordered pane stays a
	// stable rectangle — matching the old viewport.View() behavior.
	style := lipgloss.NewStyle().Width(v.width).MaxWidth(v.width)
	for i, r := range rows {
		rows[i] = style.Render(r)
	}
	return strings.Join(rows, "\n")
}
