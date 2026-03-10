// Package client provides an HTTP client for interacting with the backlog manager server.
package client

import (
	"bufio"
	"bytes"
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

// ListFeatures calls GET /api/v1/features and returns the parsed response.
// statusFilter is an optional comma-separated list of statuses (e.g. "draft,awaiting_client").
func (c *Client) ListFeatures(statusFilter string) ([]map[string]any, error) {
	url := c.BaseURL + "/api/v1/features"
	if statusFilter != "" {
		url += "?status=" + statusFilter
	}
	req, err := http.NewRequest("GET", url, nil)
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
	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// GetFeatureDetail calls GET /api/v1/features/{id} and returns the parsed response.
func (c *Client) GetFeatureDetail(featureID string) (map[string]any, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/v1/features/"+featureID, nil)
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
	return result, nil
}

// Poll calls GET /api/v1/poll and returns the action JSON on work available, or nil on timeout (204).
func (c *Client) Poll() (map[string]any, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/v1/poll", nil)
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
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
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
	return result, nil
}

// Claim calls GET /api/v1/claim and returns the action JSON on work available (with the feature
// atomically claimed), or nil on timeout (204).
func (c *Client) Claim() (map[string]any, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/v1/claim", nil)
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
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
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
	return result, nil
}

// FetchPending calls GET /api/v1/features/{id}/pending and returns the parsed response.
func (c *Client) FetchPending(featureID string) (map[string]any, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/v1/features/"+featureID+"/pending", nil)
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
	return result, nil
}

// SubmitDialog calls POST /api/v1/features/{id}/submit-dialog with the given description and questions.
func (c *Client) SubmitDialog(featureID, updatedDescription, questions string) (map[string]any, error) {
	body := map[string]string{
		"updated_description": updatedDescription,
		"questions":           questions,
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", c.BaseURL+"/api/v1/features/"+featureID+"/submit-dialog", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
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
	return result, nil
}

// StartGenerate calls POST /api/v1/features/{id}/start-generate.
func (c *Client) StartGenerate(featureID string) (map[string]any, error) {
	return c.postFeatureAction(featureID, "start-generate", nil)
}

// RegisterBead calls POST /api/v1/features/{id}/register-bead with a single bead ID.
func (c *Client) RegisterBead(featureID, beadID string) (map[string]any, error) {
	return c.postFeatureAction(featureID, "register-bead", map[string]any{"bead_id": beadID})
}

// BeadsDone calls POST /api/v1/features/{id}/beads-done to finalize bead registration.
func (c *Client) BeadsDone(featureID string) (map[string]any, error) {
	return c.postFeatureAction(featureID, "beads-done", nil)
}

// RegisterArtifact calls POST /api/v1/features/{id}/register-artifact.
// Returns nil on success (204 No Content).
func (c *Client) RegisterArtifact(featureID, artifactType, content string) error {
	body := map[string]string{"type": artifactType, "content": content}
	data, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", c.BaseURL+"/api/v1/features/"+featureID+"/register-artifact", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		if msg, ok := errResp["error"]; ok {
			return fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
		}
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// CompleteFeature calls POST /api/v1/features/{id}/complete.
func (c *Client) CompleteFeature(featureID string) (map[string]any, error) {
	return c.postFeatureAction(featureID, "complete", nil)
}

// postFeatureAction posts to /api/v1/features/{id}/{action} with optional JSON body.
func (c *Client) postFeatureAction(featureID, action string, body any) (map[string]any, error) {
	var reqBody *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequest("POST", c.BaseURL+"/api/v1/features/"+featureID+"/"+action, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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
