package tilt

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// hostPort splits an httptest server URL ("http://127.0.0.1:PORT") into host and
// port for NewClient.
func hostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return u.Hostname(), port
}

// TestWatchViewAuthAndAccumulate stands up a fake Tilt: it serves the CSRF token
// then upgrades /ws/view and streams an initial snapshot followed by a delta.
// It verifies the client fetches the token, passes it as ?csrf=, and that the
// accumulator folds the snapshot + delta into the expected merged view.
func TestWatchViewAuthAndAccumulate(t *testing.T) {
	snapshot := View{
		IsComplete:  true,
		UIResources: []UIResource{res("api", 1, "ok")},
		LogList: LogList{
			Spans:          map[string]LogSpan{"s1": {ManifestName: "api"}},
			Segments:       []LogSegment{seg("s1", "hello\n")},
			FromCheckpoint: 0, ToCheckpoint: 1,
		},
	}
	delta := View{
		UIResources: []UIResource{res("worker", 2, "error")},
		LogList: LogList{
			Segments:       []LogSegment{seg("s1", "world\n")},
			FromCheckpoint: 1, ToCheckpoint: 2,
		},
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/websocket_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "tok123")
	})
	mux.HandleFunc("/ws/view", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("csrf") != "tok123" {
			http.Error(w, "bad csrf", http.StatusForbidden)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteJSON(snapshot)
		_ = c.WriteJSON(delta)
		// Hold the connection open until the client hangs up.
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	host, port := hostPort(t, srv.URL)
	client := NewClient(host, port, "session-token")
	acc := NewViewAccumulator()

	got := make(chan *View, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = client.WatchView(ctx, func(v *View) error {
			acc.Apply(v)
			got <- acc.Snapshot()
			return nil
		})
	}()

	var last *View
	for i := 0; i < 2; i++ {
		select {
		case last = <-got:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for streamed view")
		}
	}
	cancel()

	if got := len(last.LogList.Segments); got != 2 {
		t.Fatalf("segments: got %d, want 2 (snapshot + delta appended)", got)
	}
	if last.LogList.Segments[1].Text != "world\n" {
		t.Fatalf("delta segment not appended: got %q", last.LogList.Segments[1].Text)
	}
	if got := names(last.UIResources); len(got) != 2 || got[0] != "api" || got[1] != "worker" {
		t.Fatalf("resources: got %v, want [api worker]", got)
	}
}

// TestWatchViewTokenError surfaces a failed CSRF fetch as an error (so the hub
// backs off and retries) rather than dialing with no token.
func TestWatchViewTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	host, port := hostPort(t, srv.URL)
	client := NewClient(host, port, "")
	err := client.WatchView(context.Background(), func(*View) error { return nil })
	if err == nil {
		t.Fatal("expected an error when the csrf token fetch fails")
	}
}
