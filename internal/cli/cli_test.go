package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func makeFeatureServer(t *testing.T, featureID, action string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/api/v1/features/" + featureID + "/" + action
		if r.URL.Path != expected {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}))
}

func TestStartGenerateCmd(t *testing.T) {
	ts := makeFeatureServer(t, "ft-abc", "start-generate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "ft-abc", "status": "generating"})
	})
	defer ts.Close()

	os.Setenv("BM_URL", ts.URL)
	os.Setenv("BM_TOKEN", "tok")
	defer os.Unsetenv("BM_URL")
	defer os.Unsetenv("BM_TOKEN")

	var out bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs([]string{"start-generate", "ft-abc"})
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("expected JSON output: %s, err: %v", out.String(), err)
	}
	if result["status"] != "generating" {
		t.Errorf("expected status generating, got %v", result["status"])
	}
}

func TestStartGenerateCmd_NoToken(t *testing.T) {
	os.Unsetenv("BM_TOKEN")
	os.Unsetenv("BM_URL")
	root := cli.NewRootCmd()
	root.SetArgs([]string{"start-generate", "ft-abc"})
	if err := root.Execute(); err == nil {
		t.Error("expected error when BM_TOKEN not set")
	}
}

func TestRegisterBeadsCmd(t *testing.T) {
	ts := makeFeatureServer(t, "ft-abc", "register-beads", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		ids, _ := body["bead_ids"].([]any)
		if len(ids) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "ft-abc", "status": "beads_created", "bead_ids": ids})
	})
	defer ts.Close()

	os.Setenv("BM_URL", ts.URL)
	os.Setenv("BM_TOKEN", "tok")
	defer os.Unsetenv("BM_URL")
	defer os.Unsetenv("BM_TOKEN")

	var out bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs([]string{"register-beads", "ft-abc", "bd-111", "bd-222"})
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("expected JSON output: %s, err: %v", out.String(), err)
	}
	if result["status"] != "beads_created" {
		t.Errorf("expected status beads_created, got %v", result["status"])
	}
}

func TestRegisterArtifactCmd(t *testing.T) {
	ts := makeFeatureServer(t, "ft-abc", "register-artifact", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["type"] != "plan" || body["content"] == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer ts.Close()

	os.Setenv("BM_URL", ts.URL)
	os.Setenv("BM_TOKEN", "tok")
	defer os.Unsetenv("BM_URL")
	defer os.Unsetenv("BM_TOKEN")

	// Write temp artifact file
	dir := t.TempDir()
	f := filepath.Join(dir, "plan.md")
	os.WriteFile(f, []byte("# Plan\nDo stuff."), 0644)

	var out bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs([]string{"register-artifact", "ft-abc", "--type", "plan", "--file", f})
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompleteCmd(t *testing.T) {
	ts := makeFeatureServer(t, "ft-abc", "complete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "ft-abc", "status": "done"})
	})
	defer ts.Close()

	os.Setenv("BM_URL", ts.URL)
	os.Setenv("BM_TOKEN", "tok")
	defer os.Unsetenv("BM_URL")
	defer os.Unsetenv("BM_TOKEN")

	var out bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs([]string{"complete", "ft-abc"})
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("expected JSON output: %s, err: %v", out.String(), err)
	}
	if result["status"] != "done" {
		t.Errorf("expected status done, got %v", result["status"])
	}
}
