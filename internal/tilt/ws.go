package tilt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// wsReadLimit caps a single frame. The initial /ws/view snapshot carries the
	// full log backlog, which can be several MiB on a long-running instance.
	wsReadLimit = 64 << 20 // 64 MiB
	// wsPongWait is how long a connection may be silent before we treat it as dead;
	// wsPingPeriod (< wsPongWait) is how often we ping to keep it alive and to
	// detect a half-open connection.
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 30 * time.Second
)

// WebsocketToken fetches the CSRF token required to open /ws/view. Tilt
// authenticates the websocket handshake via the ?csrf= query parameter (a
// browser cannot set headers on a WebSocket upgrade), not the X-Tilt-Token
// header, so the token must be fetched over HTTP first.
func (c *Client) WebsocketToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+"/api/websocket_token", nil)
	if err != nil {
		return "", err
	}
	if c.Token != "" {
		req.Header.Set("X-Tilt-Token", c.Token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("websocket_token: unexpected status %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// WatchView opens the /ws/view websocket and invokes onView for every streamed
// View — the initial full snapshot (IsComplete), then incremental deltas — until
// ctx is cancelled or the connection errors. It never returns nil except on ctx
// cancellation; the caller owns reconnection.
func (c *Client) WatchView(ctx context.Context, onView func(*View) error) error {
	csrf, err := c.WebsocketToken(ctx)
	if err != nil {
		return fmt.Errorf("get csrf token: %w", err)
	}

	u := url.URL{
		Scheme:   "ws",
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:     "/ws/view",
		RawQuery: "csrf=" + url.QueryEscape(csrf),
	}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.BaseURL(), err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer conn.Close()

	conn.SetReadLimit(wsReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	// Keepalive: ping periodically (resetting the read deadline on each pong) and
	// close the connection when ctx is cancelled so the blocking read below
	// unblocks. WriteControl/Close are safe to call concurrently with the reader.
	go func() {
		t := time.NewTicker(wsPingPeriod)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = conn.Close()
				return
			case <-t.C:
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read %s: %w", c.BaseURL(), err)
		}
		v, err := ParseView(data)
		if err != nil {
			return fmt.Errorf("parse view %s: %w", c.BaseURL(), err)
		}
		if err := onView(v); err != nil {
			return err
		}
	}
}
