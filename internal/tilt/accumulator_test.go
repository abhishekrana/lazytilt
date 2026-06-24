package tilt

import (
	"fmt"
	"testing"
	"time"
)

// mkSegs builds n generic, newline-terminated segments for one span.
func mkSegs(span string, n int) []LogSegment {
	out := make([]LogSegment, n)
	for i := range out {
		out[i] = LogSegment{SpanID: span, Text: fmt.Sprintf("line %d\n", i)}
	}
	return out
}

// res builds a UIResource with a name, sidebar order, and update status.
func res(name string, order int, updateStatus string) UIResource {
	return UIResource{
		Metadata: ObjectMeta{Name: name},
		Status:   UIResourceStatus{Order: order, UpdateStatus: updateStatus},
	}
}

// deletedRes builds a tombstone UIResource (deletionTimestamp set) as the
// websocket sends when a resource is removed.
func deletedRes(name string) UIResource {
	now := time.Unix(0, 0)
	return UIResource{Metadata: ObjectMeta{Name: name, DeletionTimestamp: &now}}
}

func seg(span, text string) LogSegment {
	return LogSegment{SpanID: span, Text: text}
}

func names(rs []UIResource) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name()
	}
	return out
}

func segTexts(segs []LogSegment) []string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = s.Text
	}
	return out
}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", what, got, want)
		}
	}
}

func TestAccumulatorFullThenDelta(t *testing.T) {
	a := NewViewAccumulator()

	a.Apply(&View{
		IsComplete:  true,
		UIResources: []UIResource{res("worker", 2, "ok"), res("api", 1, "ok")},
		LogList: LogList{
			Spans:          map[string]LogSpan{"s1": {ManifestName: "api"}},
			Segments:       []LogSegment{seg("s1", "first\n")},
			FromCheckpoint: 0, ToCheckpoint: 1,
		},
	})
	snap := a.Snapshot()
	// Resources sorted by order, not arrival order.
	eq(t, "resources", names(snap.UIResources), []string{"api", "worker"})
	eq(t, "segments", segTexts(snap.LogList.Segments), []string{"first\n"})

	// Delta: one changed resource + one new segment.
	a.Apply(&View{
		UIResources: []UIResource{res("worker", 2, "error")},
		LogList: LogList{
			Segments:       []LogSegment{seg("s1", "second\n")},
			FromCheckpoint: 1, ToCheckpoint: 2,
		},
	})
	snap = a.Snapshot()
	eq(t, "resources after delta", names(snap.UIResources), []string{"api", "worker"})
	eq(t, "segments after delta", segTexts(snap.LogList.Segments), []string{"first\n", "second\n"})
	if got := snap.Resources()[1].Status.UpdateStatus; got != "error" {
		t.Fatalf("worker status not merged: got %q", got)
	}
}

func TestAccumulatorDeletion(t *testing.T) {
	a := NewViewAccumulator()
	a.Apply(&View{IsComplete: true, UIResources: []UIResource{res("api", 1, "ok"), res("worker", 2, "ok")}})

	a.Apply(&View{UIResources: []UIResource{deletedRes("worker")}})

	eq(t, "after deletion", names(a.Snapshot().UIResources), []string{"api"})
}

func TestAccumulatorNoLogSentinel(t *testing.T) {
	a := NewViewAccumulator()
	a.Apply(&View{
		IsComplete:  true,
		UIResources: []UIResource{res("api", 1, "ok")},
		LogList:     LogList{Segments: []LogSegment{seg("s1", "first\n")}, FromCheckpoint: 0, ToCheckpoint: 1},
	})
	// Delta with the -1/-1 "no new logs" sentinel and only a status change.
	a.Apply(&View{
		UIResources: []UIResource{res("api", 1, "error")},
		LogList:     LogList{FromCheckpoint: -1, ToCheckpoint: -1},
	})
	snap := a.Snapshot()
	eq(t, "segments unchanged", segTexts(snap.LogList.Segments), []string{"first\n"})
	if got := snap.UIResources[0].Status.UpdateStatus; got != "error" {
		t.Fatalf("status not merged: got %q", got)
	}
}

func TestAccumulatorResetOnComplete(t *testing.T) {
	a := NewViewAccumulator()
	a.Apply(&View{
		IsComplete:  true,
		UIResources: []UIResource{res("api", 1, "ok")},
		LogList:     LogList{Segments: []LogSegment{seg("s1", "old\n")}, ToCheckpoint: 1},
	})
	a.Apply(&View{LogList: LogList{Segments: []LogSegment{seg("s1", "more\n")}, FromCheckpoint: 1, ToCheckpoint: 2}})

	// A second IsComplete (as on reconnect / server restart) wipes prior state.
	a.Apply(&View{
		IsComplete:  true,
		UIResources: []UIResource{res("db", 1, "ok")},
		LogList:     LogList{Segments: []LogSegment{seg("s2", "fresh\n")}, ToCheckpoint: 1},
	})
	snap := a.Snapshot()
	eq(t, "resources after reset", names(snap.UIResources), []string{"db"})
	eq(t, "segments after reset", segTexts(snap.LogList.Segments), []string{"fresh\n"})
}

func TestAccumulatorOrderThenName(t *testing.T) {
	a := NewViewAccumulator()
	a.Apply(&View{IsComplete: true, UIResources: []UIResource{
		res("zeta", 5, "ok"),
		res("alpha", 5, "ok"),
		res("(Tiltfile)", 0, "ok"),
	}})
	eq(t, "order then name", names(a.Snapshot().UIResources), []string{"(Tiltfile)", "alpha", "zeta"})
}

func TestAccumulatorSessionReplace(t *testing.T) {
	a := NewViewAccumulator()
	a.Apply(&View{IsComplete: true, UISession: session("v1")})
	a.Apply(&View{UISession: session("v2")})
	if got := a.Snapshot().Version(); got != "v2" {
		t.Fatalf("session not replaced: got %q", got)
	}
	// A delta without a session leaves the accumulated one intact.
	a.Apply(&View{UIResources: []UIResource{res("api", 1, "ok")}})
	if got := a.Snapshot().Version(); got != "v2" {
		t.Fatalf("session dropped by log-only delta: got %q", got)
	}
}

func session(version string) *UISession {
	s := &UISession{}
	s.Status.RunningTiltBuild.Version = version
	return s
}

// TestSnapshotIsImmutable guards the shared-slice invariant: a snapshot taken
// before later deltas must not observe segments appended afterwards.
func TestSnapshotIsImmutable(t *testing.T) {
	a := NewViewAccumulator()
	a.Apply(&View{IsComplete: true, LogList: LogList{Segments: []LogSegment{seg("s1", "a\n")}, ToCheckpoint: 1}})
	snap := a.Snapshot()
	a.Apply(&View{LogList: LogList{Segments: []LogSegment{seg("s1", "b\n")}, FromCheckpoint: 1, ToCheckpoint: 2}})
	if got := len(snap.LogList.Segments); got != 1 {
		t.Fatalf("earlier snapshot mutated by later delta: got %d segments, want 1", got)
	}
}

// The retained log buffer is capped to the most recent maxLogSegments, which
// bounds memory and per-frame render cost; spans still resolve after trimming.
func TestAccumulatorCapsRetainedSegments(t *testing.T) {
	a := NewViewAccumulator()
	over := maxLogSegments + 500
	a.Apply(&View{
		IsComplete: true,
		LogList: LogList{
			Spans:        map[string]LogSpan{"s1": {ManifestName: "api"}},
			Segments:     mkSegs("s1", over),
			ToCheckpoint: int32(over),
		},
	})
	snap := a.Snapshot()
	if got := len(snap.LogList.Segments); got != maxLogSegments {
		t.Fatalf("segments not capped: got %d, want %d", got, maxLogSegments)
	}
	// The kept segments are the most recent ones.
	if want, got := fmt.Sprintf("line %d\n", over-1), snap.LogList.Segments[maxLogSegments-1].Text; got != want {
		t.Fatalf("did not keep the newest segments: last=%q want %q", got, want)
	}
	// Spans still resolve after the trim (the full spans map is always current).
	if got := len(snap.LogList.SegmentsFor("api")); got != maxLogSegments {
		t.Fatalf("SegmentsFor after cap: got %d, want %d", got, maxLogSegments)
	}
}

// Trimming must not mutate a Snapshot taken before the trim (the shared-array
// invariant): the trim copies into a fresh backing array.
func TestAccumulatorTrimKeepsEarlierSnapshotIntact(t *testing.T) {
	a := NewViewAccumulator()
	a.Apply(&View{
		IsComplete: true,
		LogList: LogList{
			Spans:        map[string]LogSpan{"s1": {ManifestName: "api"}},
			Segments:     mkSegs("s1", maxLogSegments-1),
			ToCheckpoint: int32(maxLogSegments - 1),
		},
	})
	snap := a.Snapshot()
	before := len(snap.LogList.Segments) // maxLogSegments-1

	// Append enough to push past the cap and force a trim.
	a.Apply(&View{LogList: LogList{
		Spans:          map[string]LogSpan{"s1": {ManifestName: "api"}},
		Segments:       mkSegs("s1", 100),
		FromCheckpoint: int32(maxLogSegments - 1),
		ToCheckpoint:   int32(maxLogSegments + 99),
	}})

	if got := len(a.Snapshot().LogList.Segments); got != maxLogSegments {
		t.Fatalf("expected cap %d after trim, got %d", maxLogSegments, got)
	}
	if got := len(snap.LogList.Segments); got != before {
		t.Fatalf("trim mutated an earlier snapshot: got %d, want %d", got, before)
	}
}
