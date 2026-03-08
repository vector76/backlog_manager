package beadsserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vector76/backlog_manager/internal/beadsserver"
)

func TestGetStatuses_success(t *testing.T) {
	want := map[string]string{
		"bd-x1": "closed",
		"bd-x2": "in_progress",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/beads/status" {
			http.NotFound(w, r)
			return
		}
		ids := r.URL.Query().Get("ids")
		if ids == "" {
			http.Error(w, "missing ids", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := beadsserver.New(srv.URL)
	got, err := c.GetStatuses([]string{"bd-x1", "bd-x2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for id, status := range want {
		if got[id] != status {
			t.Errorf("bead %s: want %q got %q", id, status, got[id])
		}
	}
}

func TestGetStatuses_empty(t *testing.T) {
	c := beadsserver.New("http://localhost:9999")
	got, err := c.GetStatuses(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result for empty ids, got %v", got)
	}
}

func TestGetStatuses_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := beadsserver.New(srv.URL)
	_, err := c.GetStatuses([]string{"bd-x1"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestGetStatuses_unreachable(t *testing.T) {
	c := beadsserver.New("http://127.0.0.1:1") // port 1 should be unreachable
	_, err := c.GetStatuses([]string{"bd-x1"})
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}
