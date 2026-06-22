package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
)

const pollInterval = time.Second

// tickMsg drives the poll/refresh loop.
type tickMsg struct{}

// viewMsg carries a fetched view, tagged with the port it came from so stale
// responses (after an instance switch) can be dropped.
type viewMsg struct {
	port int
	view *tilt.View
	err  error
}

// instancesMsg carries the latest discovery result.
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

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func discoverCmd() tea.Cmd {
	return func() tea.Msg { return instancesMsg{instances: discovery.Discover()} }
}

func fetchCmd(host string, port int, token string) tea.Cmd {
	return func() tea.Msg {
		c := tilt.NewClient(host, port, token)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		v, err := c.FetchView(ctx)
		return viewMsg{port: port, view: v, err: err}
	}
}

func actionCmd(kind tilt.ActionKind, resource string, port int) tea.Cmd {
	return func() tea.Msg {
		err := tilt.RunAction(kind, resource, port)
		return actionResultMsg{kind: kind, resource: resource, err: err}
	}
}

// restartAllConcurrency bounds the number of in-flight `tilt trigger`
// subprocesses, so a large instance doesn't spawn dozens at once.
const restartAllConcurrency = 8

// restartAllCmd triggers every named resource on an instance (Tilt has no bulk
// trigger, so we fan out one CLI call per resource, capped) and reports a
// summary of how many succeeded.
func restartAllCmd(names []string, port int) tea.Cmd {
	return func() tea.Msg {
		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			failed []string
			sem    = make(chan struct{}, restartAllConcurrency)
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
				text: fmt.Sprintf("restart: %d of %d failed (%s)", len(failed), len(names), strings.Join(failed, ", ")),
				err:  true,
			}
		}
		return notifyMsg{text: fmt.Sprintf("restarted %d resources ✓", len(names))}
	}
}
