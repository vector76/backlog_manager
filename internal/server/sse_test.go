package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vector76/backlog_manager/internal/config"
	"github.com/vector76/backlog_manager/internal/server"
	"github.com/vector76/backlog_manager/internal/store"
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

// --- Auth requirement tests ---

func TestDashboardSSERequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := webRequest(t, srv, "GET", "/events", "", nil)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
}

func TestFeatureSSERequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := webRequest(t, srv, "GET", "/project/p/feature/f/events", "", nil)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
}

func TestFeatureDataRequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := webRequest(t, srv, "GET", "/project/p/feature/f/data", "", nil)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
}

// --- SSE event delivery tests (use real HTTP server for streaming) ---

// sseLogin logs in via a real HTTP client with a cookie jar and returns the client.
func sseLogin(t *testing.T, baseURL string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	body := url.Values{"username": {"admin"}, "password": {"secret"}}.Encode()
	resp, err := client.Post(baseURL+"/login", "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login: expected 302, got %d", resp.StatusCode)
	}
	return client
}

func TestDashboardSSEReceivesEventOnFeatureCreate(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:              8080,
		DashboardUser:     "admin",
		DashboardPassword: "secret",
	}
	srv, hub := server.New(cfg, st)
	t.Cleanup(hub.Stop)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	// Create a project first.
	if _, err := st.CreateProject("proj", "tok1"); err != nil {
		t.Fatal(err)
	}

	client := sseLogin(t, ts.URL)

	// Open SSE stream.
	sseResp, err := client.Get(ts.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sseResp.Body.Close() })

	if ct := sseResp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	events := make(chan string, 20)
	go func() {
		scanner := bufio.NewScanner(sseResp.Body)
		for scanner.Scan() {
			events <- scanner.Text()
		}
	}()

	// Create a feature via the API to trigger a dashboard notification.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/projects/proj/features",
		strings.NewReader(`{"name":"feat1","description":"desc"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "secret")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create feature: expected 201, got %d", resp.StatusCode)
	}

	// Wait for the SSE event.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case line := <-events:
			if line == "event: update" {
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for SSE event: update")
		}
	}
}

func TestFeatureSSEReceivesEventOnStatusChange(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:              8080,
		DashboardUser:     "admin",
		DashboardPassword: "secret",
	}
	srv, hub := server.New(cfg, st)
	t.Cleanup(hub.Stop)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	// Create project and feature.
	if _, err := st.CreateProject("proj", "tok1"); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("proj", "feat1", "desc")
	if err != nil {
		t.Fatal(err)
	}

	client := sseLogin(t, ts.URL)

	// Open feature SSE stream.
	sseURL := ts.URL + "/project/proj/feature/" + f.ID + "/events"
	sseResp, err := client.Get(sseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sseResp.Body.Close() })

	if ct := sseResp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	events := make(chan string, 20)
	go func() {
		scanner := bufio.NewScanner(sseResp.Body)
		for scanner.Scan() {
			events <- scanner.Text()
		}
	}()

	// Trigger start-dialog to change feature status.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/projects/proj/features/"+f.ID+"/start-dialog", nil)
	req.SetBasicAuth("admin", "secret")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start-dialog: expected 200, got %d", resp.StatusCode)
	}

	// Wait for the SSE event.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case line := <-events:
			if line == "event: update" {
				return
			}
		case <-timeout:
			t.Fatal("timeout waiting for SSE event: update on feature stream")
		}
	}
}

// --- Poll wiring test ---

func TestPollNotifiesDashboard(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:              8080,
		DashboardUser:     "admin",
		DashboardPassword: "secret",
	}
	srv, hub := server.New(cfg, st)
	t.Cleanup(hub.Stop)

	// Create a project to obtain a valid bearer token.
	proj, err := st.CreateProject("proj", "tok1")
	if err != nil {
		t.Fatal(err)
	}

	ch := hub.SubscribeDashboard()
	defer hub.UnsubscribeDashboard(ch)

	// Trigger poll in a goroutine; the handler calls hub.NotifyDashboard() immediately.
	go func() {
		doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(proj.Token))
	}()

	select {
	case <-ch:
		// success: dashboard was notified
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: poll did not notify dashboard hub")
	}
}
