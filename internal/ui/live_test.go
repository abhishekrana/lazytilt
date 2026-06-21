package ui

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/abhishekrana/lazytilt/internal/discovery"
	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
)

// TestLiveSmoke drives the real model against actually-running Tilt instances.
// It is gated behind LAZYTILT_LIVE=1 so the normal test run stays hermetic.
//
//	LAZYTILT_LIVE=1 go test ./internal/ui -run TestLiveSmoke -v
func TestLiveSmoke(t *testing.T) {
	if os.Getenv("LAZYTILT_LIVE") != "1" {
		t.Skip("set LAZYTILT_LIVE=1 to run against live Tilt instances")
	}
	insts := discovery.Discover()
	if len(insts) == 0 {
		t.Skip("no running Tilt instances discovered")
	}
	token, _ := tilt.ReadToken()

	m := New(token, insts[0].Host, insts[0].Port, "")
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = step(m, instancesMsg{instances: insts})

	for idx := range insts {
		in := insts[idx]
		c := tilt.NewClient(in.Host, in.Port, token)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		v, err := c.FetchView(ctx)
		cancel()
		if err != nil {
			t.Errorf("fetch %s: %v", c.BaseURL(), err)
			continue
		}
		m.active = idx
		m.view = nil
		m = step(m, viewMsg{port: in.Port, view: v})
		// Land selection on a failing resource if there is one, else the first real one.
		m.selected = 0
		for i, r := range m.visible() {
			if r.State() == tilt.StatusError {
				m.selected = i
				break
			}
			if r.Backend() != "" {
				m.selected = i
			}
		}
		m.setLogs()
		t.Logf("\n===== instance %d: %s (%s) =====\n%s", idx+1, in.Label, c.BaseURL(), m.View())
	}
}
