package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/charmbracelet/x/ansi"
)

// detailLines are the always-on resource-detail rows shown between the log-pane
// header and the logs. Each row appears only when it carries something, so a
// healthy resource with no endpoints adds nothing and the logs keep their space.
// Surfaces fields lazytilt already fetches but otherwise hides: build duration,
// endpoints and labels. (Pod restarts already ride along in the header's
// RuntimeLine, and an error is flagged by the header's status glyph plus the
// logs, so neither is repeated here.)
func (m Model) detailLines(r tilt.UIResource, w int) []string {
	var lines []string

	if d, ok := r.LastBuildDuration(); ok {
		lines = append(lines, m.theme.muted().Render("build ")+formatDuration(d))
	}

	for i, e := range r.Endpoints() {
		line := m.theme.accent().Render(e.URL)
		if i == 0 {
			line += "   " + m.theme.muted().Render("o open · y copy")
		}
		lines = append(lines, line)
	}

	if names := r.LabelNames(); len(names) > 0 {
		lines = append(lines, m.theme.muted().Render("labels ")+strings.Join(names, ", "))
	}

	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "…")
	}
	return lines
}

// formatDuration renders a build duration compactly: "350ms", "1.2s", "2m3s".
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}
