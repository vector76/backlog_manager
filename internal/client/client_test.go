package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vector76/backlog_manager/internal/client"
)

func TestGetOwnProject_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/project" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": "myproject"})
	}))
	defer ts.Close()

	c := client.NewWithBaseURL(ts.URL, "mytoken")
	result, err := c.GetOwnProject()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["name"] != "myproject" {
		t.Errorf("expected name myproject, got %v", result["name"])
	}
}

func TestGetOwnProject_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
	}))
	defer ts.Close()

	c := client.NewWithBaseURL(ts.URL, "badtoken")
	_, err := c.GetOwnProject()
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestGetOwnProject_ProjectMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"name": "actualproject", "feature_count": 0})
	}))
	defer ts.Close()

	c := client.NewWithBaseURL(ts.URL, "mytoken")
	c.Project = "expectedproject"
	_, err := c.GetOwnProject()
	if err == nil {
		t.Fatal("expected error when BM_PROJECT does not match token's project")
	}
}

func TestGetOwnProject_ProjectMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"name": "myproject", "feature_count": 0})
	}))
	defer ts.Close()

	c := client.NewWithBaseURL(ts.URL, "mytoken")
	c.Project = "myproject"
	result, err := c.GetOwnProject()
	if err != nil {
		t.Fatalf("unexpected error when BM_PROJECT matches: %v", err)
	}
	if result["name"] != "myproject" {
		t.Errorf("expected name myproject, got %v", result["name"])
	}
}

func TestDotEnvFallback(t *testing.T) {
	// Write a .env file in a temp dir, then change working directory to it
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("BM_TOKEN=fromfile\nBM_URL=http://fromfile\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// The client reads .env from the current directory, so we test parseDotEnv indirectly
	// by using NewWithBaseURL (direct test of client construction uses env vars)
	// This test validates the file was parsed correctly by checking the client behavior.
	// We test parseDotEnv by calling New() with a changed working directory.
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Unset env vars so .env file is used
	os.Unsetenv("BM_URL")
	os.Unsetenv("BM_TOKEN")
	os.Unsetenv("BM_PROJECT")
	defer func() {
		os.Unsetenv("BM_URL")
		os.Unsetenv("BM_TOKEN")
		os.Unsetenv("BM_PROJECT")
	}()

	c := client.New()
	if c.BaseURL != "http://fromfile" {
		t.Errorf("expected BM_URL=http://fromfile from .env, got %q", c.BaseURL)
	}
	if c.Token != "fromfile" {
		t.Errorf("expected BM_TOKEN=fromfile from .env, got %q", c.Token)
	}
}

func TestEnvVarOverridesDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("BM_TOKEN=fromfile\nBM_URL=http://fromfile\n"), 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	os.Setenv("BM_TOKEN", "fromenv")
	os.Setenv("BM_URL", "http://fromenv")
	defer func() {
		os.Unsetenv("BM_TOKEN")
		os.Unsetenv("BM_URL")
	}()

	c := client.New()
	if c.Token != "fromenv" {
		t.Errorf("expected BM_TOKEN=fromenv (env override), got %q", c.Token)
	}
	if c.BaseURL != "http://fromenv" {
		t.Errorf("expected BM_URL=http://fromenv (env override), got %q", c.BaseURL)
	}
}
