package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/vector76/backlog_manager/internal/cli"
)

func TestStatusCmd_NoToken(t *testing.T) {
	os.Unsetenv("BM_TOKEN")
	os.Unsetenv("BM_URL")
	os.Unsetenv("BM_PROJECT")

	root := cli.NewRootCmd()
	root.SetArgs([]string{"status"})
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when BM_TOKEN is not set")
	}
}

func TestStatusCmd_ValidToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/project" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": "testproject"})
	}))
	defer ts.Close()

	os.Setenv("BM_URL", ts.URL)
	os.Setenv("BM_TOKEN", "testtoken")
	defer func() {
		os.Unsetenv("BM_URL")
		os.Unsetenv("BM_TOKEN")
	}()

	var outBuf bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs([]string{"status"})
	root.SetOut(&outBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.NewDecoder(&outBuf).Decode(&result); err != nil {
		t.Fatalf("expected JSON output, got: %s (err: %v)", outBuf.String(), err)
	}
	if result["name"] != "testproject" {
		t.Errorf("expected name testproject, got %v", result["name"])
	}
}

func TestRootCmd_HasServeAndStatus(t *testing.T) {
	root := cli.NewRootCmd()
	cmds := root.Commands()
	names := make(map[string]bool)
	for _, c := range cmds {
		names[c.Use] = true
	}
	if !names["serve"] {
		t.Error("expected 'serve' subcommand")
	}
	if !names["status"] {
		t.Error("expected 'status' subcommand")
	}
}

func TestRootCmd_HasFeaturesAndShow(t *testing.T) {
	root := cli.NewRootCmd()
	cmds := root.Commands()
	names := make(map[string]bool)
	for _, c := range cmds {
		names[c.Use] = true
	}
	if !names["features"] {
		t.Error("expected 'features' subcommand")
	}
	if !names["show <feature-id>"] {
		t.Error("expected 'show <feature-id>' subcommand")
	}
}

func TestFeaturesCmd_NoToken(t *testing.T) {
	os.Unsetenv("BM_TOKEN")
	os.Unsetenv("BM_URL")

	root := cli.NewRootCmd()
	root.SetArgs([]string{"features"})
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when BM_TOKEN is not set")
	}
}

func TestFeaturesCmd_ValidToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/features" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{
			{"id": "ft-abc", "name": "My Feature", "status": "draft"},
		})
	}))
	defer ts.Close()

	os.Setenv("BM_URL", ts.URL)
	os.Setenv("BM_TOKEN", "testtoken")
	defer func() {
		os.Unsetenv("BM_URL")
		os.Unsetenv("BM_TOKEN")
	}()

	var outBuf bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs([]string{"features"})
	root.SetOut(&outBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []map[string]any
	if err := json.NewDecoder(&outBuf).Decode(&result); err != nil {
		t.Fatalf("expected JSON array, got: %s (err: %v)", outBuf.String(), err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 feature, got %d", len(result))
	}
	if result[0]["id"] != "ft-abc" {
		t.Errorf("expected id ft-abc, got %v", result[0]["id"])
	}
}

func TestShowCmd_NoToken(t *testing.T) {
	os.Unsetenv("BM_TOKEN")
	os.Unsetenv("BM_URL")

	root := cli.NewRootCmd()
	root.SetArgs([]string{"show", "ft-abc"})
	var errBuf bytes.Buffer
	root.SetErr(&errBuf)
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when BM_TOKEN is not set")
	}
}

func TestShowCmd_ValidToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/features/ft-abc" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":                  "ft-abc",
			"name":                "My Feature",
			"status":              "draft",
			"initial_description": "# My Feature\nDescription here.",
		})
	}))
	defer ts.Close()

	os.Setenv("BM_URL", ts.URL)
	os.Setenv("BM_TOKEN", "testtoken")
	defer func() {
		os.Unsetenv("BM_URL")
		os.Unsetenv("BM_TOKEN")
	}()

	var outBuf bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs([]string{"show", "ft-abc"})
	root.SetOut(&outBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.NewDecoder(&outBuf).Decode(&result); err != nil {
		t.Fatalf("expected JSON object, got: %s (err: %v)", outBuf.String(), err)
	}
	if result["id"] != "ft-abc" {
		t.Errorf("expected id ft-abc, got %v", result["id"])
	}
	if result["initial_description"] == "" {
		t.Error("expected non-empty initial_description")
	}
}
