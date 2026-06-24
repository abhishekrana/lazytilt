package tilt

import (
	"sort"
	"time"
)

// maxLogSegments bounds the per-instance log buffer the accumulator retains. It
// caps memory (the stream is otherwise unbounded) and per-frame render cost:
// everything downstream — AllLines/SegmentsFor, line wrapping, and the
// viewport's O(total) SetContent — is then O(maxLogSegments), not O(all logs).
// It is the single CPU<->scrollback knob: lower for less CPU, higher for deeper
// history (~one segment per log line).
const maxLogSegments = 3000

// ViewAccumulator reconstructs a complete View from the incremental updates Tilt
// streams over /ws/view. The first message of a connection is a full snapshot
// (IsComplete); every later message carries only what changed: changed
// uiResources (merged by metadata.name; a non-nil deletionTimestamp means the
// resource was removed), a replaced uiSession, and new log segments (appended).
// Folding the deltas here lets the rest of lazytilt keep consuming a full
// *View exactly as it did under HTTP polling.
//
// A ViewAccumulator is not safe for concurrent use; drive it from one goroutine
// (the per-instance watcher).
type ViewAccumulator struct {
	resources map[string]UIResource
	session   *UISession
	startTime time.Time

	// spans is replaced wholesale on each log update (Tilt sends the full span
	// map every time); segments is append-only — the delta carries just the new
	// ones. A Snapshot shares the segments backing array read-only, and Apply
	// only ever appends past a prior snapshot's length, so the share is race-free.
	spans    map[string]LogSpan
	segments []LogSegment
}

// NewViewAccumulator returns an empty accumulator ready for its first Apply.
func NewViewAccumulator() *ViewAccumulator {
	return &ViewAccumulator{
		resources: map[string]UIResource{},
		spans:     map[string]LogSpan{},
	}
}

func (a *ViewAccumulator) reset() {
	a.resources = map[string]UIResource{}
	a.session = nil
	a.spans = map[string]LogSpan{}
	a.segments = nil
}

// Apply folds one streamed View — a full snapshot or a delta — into the
// accumulated state. A view with IsComplete set (the first message, and the
// first message after any reconnect) resets the state first, which also covers
// server restarts: a reconnect always begins with a fresh complete snapshot.
func (a *ViewAccumulator) Apply(v *View) {
	if v == nil {
		return
	}
	if v.IsComplete {
		a.reset()
	}
	for _, r := range v.UIResources {
		name := r.Metadata.Name
		if r.Metadata.DeletionTimestamp != nil {
			delete(a.resources, name)
			continue
		}
		a.resources[name] = r
	}
	if v.UISession != nil {
		a.session = v.UISession
	}
	if !v.TiltStartTime.IsZero() {
		a.startTime = v.TiltStartTime
	}
	// logList is present only when logs changed. FromCheckpoint == -1 is Tilt's
	// "no new logs" sentinel. Spans arrive as the full map, so replace them;
	// segments are the delta, so append.
	ll := v.LogList
	if len(ll.Spans) > 0 {
		a.spans = ll.Spans
	}
	if ll.FromCheckpoint != -1 && len(ll.Segments) > 0 {
		a.segments = append(a.segments, ll.Segments...)
		if len(a.segments) > maxLogSegments {
			// Keep the most recent maxLogSegments. Copy into a fresh array rather
			// than reslicing: prior Snapshots share the old backing array and rely
			// on it staying immutable, and the accumulator must keep appending
			// without aliasing them. The copy is ~maxLogSegments small structs.
			a.segments = append([]LogSegment(nil), a.segments[len(a.segments)-maxLogSegments:]...)
		}
	}
}

// Snapshot returns the fully-merged View. Resources are sorted by status.order
// then name (deterministic, matching Tilt's display order); the log segments are
// shared read-only with the accumulator (see the field comment).
func (a *ViewAccumulator) Snapshot() *View {
	res := make([]UIResource, 0, len(a.resources))
	for _, r := range a.resources {
		res = append(res, r)
	}
	sort.Slice(res, func(i, j int) bool {
		if oi, oj := res[i].Status.Order, res[j].Status.Order; oi != oj {
			return oi < oj
		}
		return res[i].Metadata.Name < res[j].Metadata.Name
	})
	return &View{
		UISession:     a.session,
		UIResources:   res,
		LogList:       LogList{Spans: a.spans, Segments: a.segments},
		TiltStartTime: a.startTime,
	}
}
