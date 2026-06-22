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
	}
	buf := map[string]*pending{}
	var order []string
	var out []LogLine

	for i := range l.Segments {
		s := l.Segments[i]
		p, ok := buf[s.SpanID]
		if !ok {
			p = &pending{}
			buf[s.SpanID] = p
			order = append(order, s.SpanID)
		}
		p.level = s.Level
		text := s.Text
		for {
			j := strings.IndexByte(text, '\n')
			if j < 0 {
				p.b.WriteString(text)
				break
			}
			p.b.WriteString(text[:j])
			out = append(out, LogLine{Manifest: l.ManifestOf(s.SpanID), Level: s.Level, Text: p.b.String()})
			p.b.Reset()
			text = text[j+1:]
		}
	}
	for _, span := range order {
		if p := buf[span]; p.b.Len() > 0 {
			out = append(out, LogLine{Manifest: l.ManifestOf(span), Level: p.level, Text: p.b.String()})
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
