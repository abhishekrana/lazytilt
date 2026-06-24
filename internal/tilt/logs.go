package tilt

import (
	"sort"
	"strings"
)

// ManifestOf resolves a segment's span to its owning manifest name. The empty
// string denotes global (non-resource) output.
func (l *LogList) ManifestOf(spanID string) string {
	return l.Spans[spanID].ManifestName
}

// SegmentsFor returns the log segments belonging to a single manifest, in order.
func (l *LogList) SegmentsFor(manifest string) []LogSegment {
	var out []LogSegment
	for _, s := range l.Segments {
		if l.ManifestOf(s.SpanID) == manifest {
			out = append(out, s)
		}
	}
	return out
}

// LogLine is one assembled log line tagged with the resource (manifest) it came
// from. An empty Manifest denotes global (non-resource) Tilt output.
type LogLine struct {
	Manifest string
	Level    string
	Text     string
}

// AllLines assembles every segment into lines tagged by manifest, preserving the
// global ordering — the backing data for the "All Resources" combined view. Tilt
// streams text in arbitrary chunks (a segment may be a partial line), so we
// buffer per span and flush on each newline, which keeps every emitted line
// attributed to a single resource. Trailing partial lines are emitted at the end
// in the order their spans first appeared (so the result is deterministic).
func (l *LogList) AllLines() []LogLine {
	type pending struct {
		b     strings.Builder
		level string
		last  int // most recent segment index appended to this span's buffer
	}
	type tagged struct {
		line LogLine
		idx  int // segment index this line completed (or was last written) at
	}
	buf := map[string]*pending{}
	var lines []tagged

	for i := range l.Segments {
		s := l.Segments[i]
		p, ok := buf[s.SpanID]
		if !ok {
			p = &pending{}
			buf[s.SpanID] = p
		}
		p.level = s.Level
		text := s.Text
		for {
			j := strings.IndexByte(text, '\n')
			if j < 0 {
				if text != "" {
					p.b.WriteString(text)
					p.last = i
				}
				break
			}
			p.b.WriteString(text[:j])
			lines = append(lines, tagged{LogLine{Manifest: l.ManifestOf(s.SpanID), Level: s.Level, Text: p.b.String()}, i})
			p.b.Reset()
			text = text[j+1:]
		}
	}
	// Emit each span's leftover partial (a line with no trailing newline) at the
	// segment where it was last written, so a resource that ended mid-line — e.g. a
	// build that printed "failed: exit status 1" with no newline — stays in
	// chronological order instead of being pinned below newer output from others.
	for span, p := range buf {
		if p.b.Len() > 0 {
			lines = append(lines, tagged{LogLine{Manifest: l.ManifestOf(span), Level: p.level, Text: p.b.String()}, p.last})
		}
	}
	// Stable sort by segment index. Complete lines were appended in index order, so
	// their relative order is preserved; partials slot in at their last-write index.
	// Indices are unique per line (one span per segment), so ordering is deterministic.
	sort.SliceStable(lines, func(a, b int) bool { return lines[a].idx < lines[b].idx })
	out := make([]LogLine, len(lines))
	for i := range lines {
		out[i] = lines[i].line
	}
	return out
}

// Lines turns a slice of segments into newline-terminated lines. Tilt streams
// text in arbitrary chunks (a segment may be a partial line), so we concatenate
// and split on newlines. ANSI escapes are preserved.
func Lines(segs []LogSegment) []string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	text := b.String()
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}
