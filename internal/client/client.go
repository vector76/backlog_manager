// Package client provides an HTTP client for interacting with the backlog manager server.
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Client is an HTTP client for the backlog manager API.
type Client struct {
	BaseURL    string
	Token      string
	Project    string
	HTTPClient *http.Client
}

// New creates a Client from environment variables, falling back to .env file if needed.
// BM_URL: server URL (default: http://localhost:8080)
// BM_TOKEN: bearer token (required for token-auth endpoints)
// BM_PROJECT: project name (optional)
func New() *Client {
	env := loadEnv()
	baseURL := env["BM_URL"]
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &Client{
		BaseURL:    baseURL,
		Token:      env["BM_TOKEN"],
		Project:    env["BM_PROJECT"],
		HTTPClient: &http.Client{},
	}
}

// NewWithBaseURL creates a Client with an explicit base URL and token (for testing).
func NewWithBaseURL(baseURL, token string) *Client {
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		HTTPClient: &http.Client{},
	}
}

// GetOwnProject calls GET /api/v1/project and returns the parsed response body.
func (c *Client) GetOwnProject() (map[string]any, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/v1/project", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		if msg, ok := errResp["error"]; ok {
			return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if c.Project != "" {
		if name, _ := result["name"].(string); name != c.Project {
			return nil, fmt.Errorf("token belongs to project %q, but BM_PROJECT is set to %q", name, c.Project)
		}
	}
	return result, nil
}

// loadEnv returns a map of environment variables, merging .env file as fallback.
// Environment variables take priority over .env file values.
func loadEnv() map[string]string {
	keys := []string{"BM_URL", "BM_TOKEN", "BM_PROJECT"}
	result := make(map[string]string, len(keys))

	// First load from .env file
	dotenv := parseDotEnv(".env")
	for _, k := range keys {
		if v, ok := dotenv[k]; ok {
			result[k] = v
		}
	}
	// Then override with actual environment variables
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			result[k] = v
		}
	}
	return result
}

// parseDotEnv reads a .env file and returns KEY=VALUE pairs.
// Lines starting with # are comments. Missing file returns empty map.
func parseDotEnv(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return map[string]string{}
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip optional surrounding quotes
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		result[key] = val
	}
	return result
}
