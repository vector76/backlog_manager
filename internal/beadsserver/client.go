// Package beadsserver provides an HTTP client for querying bead statuses from the beads server.
package beadsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client queries the beads server for bead statuses.
type Client struct {
	URL        string
	HTTPClient *http.Client
}

// New creates a new Client with the given base URL.
func New(url string) *Client {
	return &Client{
		URL:        strings.TrimRight(url, "/"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetStatuses queries the status of the given bead IDs.
// Returns a map of bead ID to status string (e.g. "open", "in_progress", "closed").
func (c *Client) GetStatuses(ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	url := c.URL + "/api/v1/beads/status?ids=" + strings.Join(ids, ",")
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get bead statuses: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get bead statuses: unexpected status %d", resp.StatusCode)
	}
	var statuses map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("decode bead statuses: %w", err)
	}
	return statuses, nil
}
