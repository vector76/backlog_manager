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
	"github.com/vector76/backlog_manager/internal/model"
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
	f, err := st.CreateFeature("proj1", "feat1", "desc", false, "")
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
		Name       string `json:"name"`
		Status     string `json:"status"`
		Iterations []any  `json:"iterations"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", resp.Status)
	}
	if resp.Name != "feat1" {
		t.Errorf("expected name 'feat1', got %q", resp.Name)
	}
}

// TestHandleFeatureData_OtherFeatures verifies that the /data endpoint includes
// other_features for a fully_specified feature and omits it for other statuses.
func TestHandleFeatureData_OtherFeatures(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	const proj = "of-project"
	if _, err := st.CreateProject(proj, "tok-of"); err != nil {
		t.Fatal(err)
	}

	// Create subject feature and advance to fully_specified.
	subject, err := st.CreateFeature(proj, "Subject", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, subject.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, subject.ID, model.StatusFullySpecified); err != nil {
		t.Fatal(err)
	}

	// Create a sibling that should appear in other_features.
	sibling, err := st.CreateFeature(proj, "Sibling", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, sibling.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, sibling.ID, model.StatusFullySpecified); err != nil {
		t.Fatal(err)
	}

	path := "/project/" + proj + "/feature/" + subject.ID + "/data"
	w := webRequest(t, srv, "GET", path, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Status        string `json:"status"`
		OtherFeatures []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"other_features"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "fully_specified" {
		t.Fatalf("expected status fully_specified, got %q", resp.Status)
	}
	if len(resp.OtherFeatures) != 1 {
		t.Fatalf("expected 1 other feature, got %d", len(resp.OtherFeatures))
	}
	if resp.OtherFeatures[0].ID != sibling.ID {
		t.Errorf("expected sibling ID %q, got %q", sibling.ID, resp.OtherFeatures[0].ID)
	}

	// A draft feature should have no other_features in the JSON.
	draft, err := st.CreateFeature(proj, "Draft", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	w2 := webRequest(t, srv, "GET", "/project/"+proj+"/feature/"+draft.ID+"/data", "", cookie)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	var resp2 struct {
		OtherFeatures []any `json:"other_features"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp2.OtherFeatures) != 0 {
		t.Errorf("expected no other_features for draft feature, got %d", len(resp2.OtherFeatures))
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
	f, err := st.CreateFeature("proj1", "feat1", "desc", false, "")
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
	f, err := st.CreateFeature("proj", "feat1", "desc", false, "")
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

// TestHandleFeatureData_beadsCreated_cacheMiss verifies that the /data endpoint
// returns bead_progress with correct counts for a beads_created feature when no
// BeadMonitor is present (cache miss / unavailable=false, counts from store).
func TestHandleFeatureData_beadsCreated_cacheMiss(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtGenerating(t, srv, st, "bddata")
	token := tokenForProject(t, st, "bddata")
	cookie := loginWeb(t, srv)

	// Register two beads.
	for _, beadID := range []string{"bd-aaa1", "bd-bbb2"} {
		w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-bead",
			map[string]any{"bead_id": beadID}, bearerAuth(token))
		if w.Code != http.StatusOK {
			t.Fatalf("register-bead %s: expected 200, got %d: %s", beadID, w.Code, w.Body.String())
		}
	}

	// Transition to beads_created.
	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/beads-done", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("beads-done: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// GET the feature data endpoint via web session.
	path := "/project/bddata/feature/" + featureID + "/data"
	w2 := webRequest(t, srv, "GET", path, "", cookie)
	if w2.Code != http.StatusOK {
		t.Fatalf("feature data: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp struct {
		BeadProgress *struct {
			Total       int  `json:"total"`
			Closed      int  `json:"closed"`
			Unavailable bool `json:"unavailable"`
		} `json:"bead_progress"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.BeadProgress == nil {
		t.Fatal("expected bead_progress to be present, got nil (omitted)")
	}
	if resp.BeadProgress.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.BeadProgress.Total)
	}
	if resp.BeadProgress.Closed != 0 {
		t.Errorf("expected closed=0, got %d", resp.BeadProgress.Closed)
	}
	if resp.BeadProgress.Unavailable {
		t.Error("expected unavailable=false, got true")
	}
}

// TestHandleDashboardDataUpdatedAtISO checks that the /data endpoint includes
// updated_at_iso (ISO 8601) alongside updated_at for each feature row.
func TestHandleDashboardDataUpdatedAtISO(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("iso-project", "tok-iso")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFeature("iso-project", "ISO Feature", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/data", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Projects []struct {
			Features []struct {
				UpdatedAt    string `json:"updated_at"`
				UpdatedAtISO string `json:"updated_at_iso"`
			} `json:"features"`
		} `json:"projects"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Projects) == 0 || len(resp.Projects[0].Features) == 0 {
		t.Fatal("expected at least one feature in response")
	}
	f := resp.Projects[0].Features[0]
	if f.UpdatedAtISO == "" {
		t.Error("expected non-empty updated_at_iso in /data response")
	}
	// Must be parseable as RFC3339.
	if _, err := time.Parse(time.RFC3339, f.UpdatedAtISO); err != nil {
		t.Errorf("updated_at_iso %q is not valid RFC3339: %v", f.UpdatedAtISO, err)
	}
	// Display string should contain "UTC" suffix.
	if !strings.Contains(f.UpdatedAt, "UTC") {
		t.Errorf("expected updated_at to contain 'UTC', got %q", f.UpdatedAt)
	}
}
