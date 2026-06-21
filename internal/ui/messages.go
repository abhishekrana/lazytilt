package ui

import (
	"context"
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
