package tilt

import "testing"

func lineTexts(ls []LogLine) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Text
	}
	return out
}

// A span that ends mid-line (no trailing newline) must stay in chronological
// order, not get pinned below newer complete lines from other spans — the bug
// where a failed build's "failed: exit status 1" stuck to the bottom of the
// combined "All Resources" view while other resources kept streaming.
func TestAllLinesDanglingPartialStaysChronological(t *testing.T) {
	ll := LogList{
		Spans: map[string]LogSpan{"a": {ManifestName: "worker"}, "b": {ManifestName: "api"}},
		Segments: []LogSegment{
			seg("a", "worker up\n"),
			seg("a", "build failed"), // dangling: no trailing newline, never completed
			seg("b", "api line 1\n"),
			seg("b", "api line 2\n"),
		},
	}

	got := lineTexts(ll.AllLines())
	want := []string{"worker up", "build failed", "api line 1", "api line 2"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// Several spans can each end mid-line; every dangling partial must land at its own
// chronological spot, not bunch up at the bottom while a live span streams above.
func TestAllLinesMultipleDanglingPartials(t *testing.T) {
	ll := LogList{
		Spans: map[string]LogSpan{
			"a": {ManifestName: "worker"},
			"b": {ManifestName: "db"},
			"c": {ManifestName: "api"},
		},
		Segments: []LogSegment{
			seg("a", "worker died"), // dangling (idx 0)
			seg("b", "db died"),     // dangling (idx 1)
			seg("c", "api 1\n"),     // live span keeps streaming complete lines
			seg("c", "api 2\n"),
			seg("c", "api 3\n"),
		},
	}

	got := lineTexts(ll.AllLines())
	want := []string{"worker died", "db died", "api 1", "api 2", "api 3"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// Multi-segment lines interrupted by another span must still reassemble per span.
func TestAllLinesReassemblesInterleavedSegments(t *testing.T) {
	ll := LogList{
		Spans: map[string]LogSpan{"a": {ManifestName: "worker"}, "b": {ManifestName: "api"}},
		Segments: []LogSegment{
			seg("a", "hel"),
			seg("b", "api line\n"),
			seg("a", "lo\n"), // completes "hello" for span a despite b's interjection
		},
	}

	got := lineTexts(ll.AllLines())
	want := []string{"api line", "hello"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}
