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
// Surfaces fields lazytilt already fetches but otherwise hides: build error,
// build warnings, workload kind, build duration, endpoints and labels. (Pod
// restarts already ride along in the header's RuntimeLine, so they aren't repeated.)
func (m Model) detailLines(r tilt.UIResource, w int) []string {
	var lines []string

	// When a build failed, its error is only in the build-log span — which the
	// running pod's own stdout buries in the pane. Surface the failure summary here
	// so a ✕ resource says *what* broke, not just that it did.
	if e := firstLine(r.LastError()); e != "" {
		lines = append(lines, m.theme.err().Render("error ")+e)
	}

	// Warnings don't set the resource's error state, so a green resource can carry
	// them — show the first, tagged with the count when there's more than one.
	if w := r.LastWarnings(); len(w) > 0 {
		if msg := firstLine(strings.Join(w, "\n")); msg != "" {
			label := "warn "
			if len(w) > 1 {
				label = fmt.Sprintf("warn %d ", len(w))
			}
			lines = append(lines, m.theme.warn().Render(label)+msg)
		}
	}

	if kinds := r.WorkloadKinds(); len(kinds) > 0 {
		lines = append(lines, m.theme.muted().Render("kind ")+strings.Join(kinds, ", "))
	}

	if d, ok := r.LastBuildDuration(); ok {
		lines = append(lines, m.theme.muted().Render("build ")+formatDuration(d))
	}

	for _, e := range r.Endpoints() {
		lines = append(lines, m.theme.accent().Render(e.URL))
	}

	if names := r.LabelNames(); len(names) > 0 {
		lines = append(lines, m.theme.muted().Render("labels ")+strings.Join(names, ", "))
	}

	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "…")
	}
	return lines
}

// firstLine returns the first non-empty line of s, sanitized and ANSI-stripped so
// a multi-line build error collapses to its summary row and can't corrupt the strip.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ansi.Strip(sanitizeLogLine(ln))); t != "" {
			return t
		}
	}
	return ""
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
