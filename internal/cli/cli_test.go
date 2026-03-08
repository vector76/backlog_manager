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
