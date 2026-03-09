package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleDashboardData(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	// No projects initially.
	w := webRequest(t, srv, "GET", "/data", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["projects"]; !ok {
		t.Error("expected 'projects' key in response")
	}

	// Create a project and verify it appears.
	_, err := st.CreateProject("alpha", "tok1")
	if err != nil {
		t.Fatal(err)
	}

	w2 := webRequest(t, srv, "GET", "/data", "", cookie)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	var resp2 struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp2.Projects) != 1 || resp2.Projects[0].Name != "alpha" {
		t.Errorf("expected project 'alpha', got %+v", resp2.Projects)
	}
}

func TestHandleFeatureData(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	// Create project and feature.
	_, err := st.CreateProject("proj1", "tok1")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("proj1", "feat1", "desc")
	if err != nil {
		t.Fatal(err)
	}

	path := "/project/proj1/feature/" + f.ID + "/data"
	w := webRequest(t, srv, "GET", path, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
	var resp struct {
		Status     string `json:"status"`
		Iterations []any  `json:"iterations"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", resp.Status)
	}
}

func TestHandleFeatureData_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	w := webRequest(t, srv, "GET", "/project/noproject/feature/noid/data", "", cookie)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDashboardData_RequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)

	// No cookie — should redirect to login.
	w := webRequest(t, srv, "GET", "/data", "", nil)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
}

func TestHandleDashboardSSE(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	// Use a cancellable context so the SSE handler terminates.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/events", nil).WithContext(ctx)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}
}

func TestHandleFeatureSSE(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("proj1", "tok1")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("proj1", "feat1", "desc")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	path := "/project/proj1/feature/" + f.ID + "/events"
	req := httptest.NewRequest("GET", path, nil).WithContext(ctx)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}
}
