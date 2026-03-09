package beadsserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestSubscribeSSE_eventReceived(t *testing.T) {
	ready := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: update\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-ready // block to keep connection open
	}))
	defer srv.Close()
	defer close(ready)

	c := beadsserver.New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.SubscribeSSE(ctx)

	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly before event")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for SSE event")
	}
}

func TestSubscribeSSE_eofSignalsDrop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close immediately
	}))
	defer srv.Close()

	c := beadsserver.New(srv.URL)
	ch := c.SubscribeSSE(context.Background())

	select {
	case _, ok := <-ch:
		if ok {
			// consumed an event; channel should close next
			select {
			case _, ok2 := <-ch:
				if ok2 {
					t.Fatal("channel not closed after EOF")
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("timed out waiting for channel close after EOF")
			}
		}
		// ok==false means closed — that's what we want
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for channel close after EOF")
	}
}

func TestSubscribeSSE_unreachable(t *testing.T) {
	c := beadsserver.New("http://127.0.0.1:1")
	ch := c.SubscribeSSE(context.Background())

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed, got event")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for drop signal from unreachable server")
	}
}

func TestSubscribeSSE_cancelStopsReader(t *testing.T) {
	stopped := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		close(stopped)
	}))
	defer srv.Close()

	c := beadsserver.New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	ch := c.SubscribeSSE(ctx)

	// Give SSE connection time to establish
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Channel should close promptly
	select {
	case _, ok := <-ch:
		if ok {
			// drain any pending events
			select {
			case <-ch:
			case <-time.After(500 * time.Millisecond):
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for channel close after cancel")
	}

	// Verify server-side request was cancelled
	select {
	case <-stopped:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for server to see cancel")
	}
}
