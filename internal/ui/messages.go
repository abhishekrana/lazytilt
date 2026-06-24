package ui

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
)

// viewMsg carries a merged view snapshot for one instance (or a stream error),
// tagged with the port it came from so a stale instance's data can be dropped.
// The Hub pushes these onto the events channel as websocket deltas arrive.
type viewMsg struct {
	port int
	view *tilt.View
	err  error
}

// instancesMsg carries the latest discovery result (pushed by the Hub).
type instancesMsg struct {
	instances []discovery.Instance
}

// actionResultMsg carries the outcome of a trigger/enable/disable.
type actionResultMsg struct {
	kind     tilt.ActionKind
	resource string
	err      error
}

// notifyMsg carries a one-off status line (e.g. the result of opening or copying
// an endpoint) to surface in the footer.
type notifyMsg struct {
	text string
	err  bool
}

// listenCmd waits for the next message the Hub pushes onto the events channel.
// The Update loop re-arms it after each hub-sourced message, so exactly one read
// is ever outstanding — this is what drives the UI now that polling is gone.
func listenCmd(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func actionCmd(kind tilt.ActionKind, resource string, port int) tea.Cmd {
	return func() tea.Msg {
		err := tilt.RunAction(kind, resource, port)
		return actionResultMsg{kind: kind, resource: resource, err: err}
	}
}

// triggerAllConcurrency bounds the number of in-flight `tilt trigger`
// subprocesses, so a large instance doesn't spawn dozens at once.
const triggerAllConcurrency = 8

// triggerAllCmd triggers every named resource on an instance (Tilt has no bulk
// trigger, so we fan out one CLI call per resource, capped) and reports a
// summary of how many succeeded.
func triggerAllCmd(names []string, port int) tea.Cmd {
	return func() tea.Msg {
		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			failed []string
			sem    = make(chan struct{}, triggerAllConcurrency)
		)
		for _, n := range names {
			wg.Add(1)
			go func(n string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if err := tilt.RunAction(tilt.ActionTrigger, n, port); err != nil {
					mu.Lock()
					failed = append(failed, n)
					mu.Unlock()
				}
			}(n)
		}
		wg.Wait()
		if len(failed) > 0 {
			sort.Strings(failed)
			return notifyMsg{
				text: fmt.Sprintf("trigger: %d of %d failed (%s)", len(failed), len(names), strings.Join(failed, ", ")),
				err:  true,
			}
		}
		return notifyMsg{text: fmt.Sprintf("triggered %d resources ✓", len(names))}
	}
}
