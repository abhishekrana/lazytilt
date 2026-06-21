package tilt

import "strings"

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
