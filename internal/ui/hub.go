package ui

import (
	"context"
	"time"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	// discoveryInterval is how often the hub rescans for running Tilt instances
	// (a /proc scan of a few ms) to open/close websockets as instances come and go.
	discoveryInterval = 2 * time.Second
	// websocket reconnect backoff bounds, doubling from min to max.
	wsReconnectMin = 1 * time.Second
	wsReconnectMax = 30 * time.Second
)

// Hub is the single I/O owner. It discovers running Tilt instances and holds one
// /ws/view websocket per instance, folding the streamed deltas into a
// per-instance accumulator and pushing fully-merged viewMsg snapshots (plus
// instancesMsg) onto the events channel the UI listens on. It replaces the old
// once-a-second HTTP polling of every instance's full /api/view: the UI now does
// work only when something actually changes, and idle CPU drops to ~zero.
type Hub struct {
	token        string
	fallbackHost string
	fallbackPort int
	events       chan<- tea.Msg

	conns map[int]context.CancelFunc // port -> cancel for its watcher goroutine
}

// NewHub builds a hub. host/port are the fallback instance used when discovery
// finds nothing (matching the UI's fallback). events is the channel ui.Model
// listens on (see Model.Events).
func NewHub(token, host string, port int, events chan<- tea.Msg) *Hub {
	return &Hub{
		token:        token,
		fallbackHost: host,
		fallbackPort: port,
		events:       events,
		conns:        map[int]context.CancelFunc{},
	}
}

// Run discovers instances and reconciles websocket connections until ctx is
// cancelled. It blocks; run it in a goroutine.
func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(discoveryInterval)
	defer t.Stop()
	h.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.reconcile(ctx)
		}
	}
}

// reconcile runs discovery, emits the instance list, and opens/closes watchers
// so the live set of connections matches the discovered set. Mirrors the stale-
// instance pruning the UI used to do in handleInstances.
func (h *Hub) reconcile(ctx context.Context) {
	insts := discovery.Discover()
	if len(insts) == 0 && h.fallbackPort != 0 {
		insts = []discovery.Instance{{Host: h.fallbackHost, Port: h.fallbackPort, Label: h.fallbackHost}}
	}
	h.emit(ctx, instancesMsg{instances: insts})

	want := make(map[int]discovery.Instance, len(insts))
	for _, in := range insts {
		want[in.Port] = in
	}
	for port, in := range want {
		if _, ok := h.conns[port]; !ok {
			cctx, cancel := context.WithCancel(ctx)
			h.conns[port] = cancel
			go h.watch(cctx, in)
		}
	}
	for port, cancel := range h.conns {
		if _, ok := want[port]; !ok {
			cancel()
			delete(h.conns, port)
		}
	}
}

// watch maintains one instance's websocket, reconnecting with capped backoff. A
// fresh connection always begins with an IsComplete snapshot, so the
// accumulator resets itself on every reconnect (and on a server restart).
func (h *Hub) watch(ctx context.Context, in discovery.Instance) {
	c := tilt.NewClient(in.Host, in.Port, h.token)
	acc := tilt.NewViewAccumulator()
	backoff := wsReconnectMin
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.WatchView(ctx, func(v *tilt.View) error {
			acc.Apply(v)
			h.emit(ctx, viewMsg{port: in.Port, view: acc.Snapshot()})
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			h.emit(ctx, viewMsg{port: in.Port, err: err})
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > wsReconnectMax {
			backoff = wsReconnectMax
		}
	}
}

// emit sends a message to the UI, abandoning the send if ctx is cancelled (on
// shutdown or when this instance's watcher is being torn down) so goroutines
// never leak on a channel nobody is reading.
func (h *Hub) emit(ctx context.Context, msg tea.Msg) {
	select {
	case h.events <- msg:
	case <-ctx.Done():
	}
}
