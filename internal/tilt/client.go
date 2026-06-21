package tilt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to a single running Tilt instance's web API.
type Client struct {
	Host  string
	Port  int
	Token string
	http  *http.Client
}

// NewClient builds a client for host:port. token may be empty (Tilt < 0.37
// serves the view unauthenticated; newer versions require it).
func NewClient(host string, port int, token string) *Client {
	return &Client{
		Host:  host,
		Port:  port,
		Token: token,
		http:  &http.Client{Timeout: 5 * time.Second},
	}
}

// BaseURL is the http origin for the instance, e.g. http://localhost:10350.
func (c *Client) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.Host, c.Port)
}

// FetchView retrieves and decodes GET /api/view.
func (c *Client) FetchView(ctx context.Context) (*View, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+"/api/view", nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("X-Tilt-Token", c.Token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tilt %s: unexpected status %s", c.BaseURL(), resp.Status)
	}
	var v View
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("decode view: %w", err)
	}
	return &v, nil
}

// ParseView decodes a View from raw JSON (the same shape GET /api/view returns).
func ParseView(b []byte) (*View, error) {
	var v View
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// ReadToken reads the local Tilt API token from ~/.tilt-dev/token. A missing
// token is not an error (returns "").
func ReadToken() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(home, ".tilt-dev", "token"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
