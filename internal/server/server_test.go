package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vector76/backlog_manager/internal/config"
	"github.com/vector76/backlog_manager/internal/server"
	"github.com/vector76/backlog_manager/internal/store"
)

func newTestServer(t *testing.T) (*http.Server, *store.Store) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:              8080,
		DataDir:           t.TempDir(),
		DashboardUser:     "admin",
		DashboardPassword: "secret",
	}
	srv := server.New(cfg, st)
	return srv, st
}

func doRequest(t *testing.T, srv *http.Server, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	return w
}

func basicAuth(user, pass string) map[string]string {
	req, _ := http.NewRequest("GET", "/", nil)
	req.SetBasicAuth(user, pass)
	return map[string]string{"Authorization": req.Header.Get("Authorization")}
}

func bearerAuth(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// --- Health / Version ---

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/health", nil, nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestVersion(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/version", nil, nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["version"] == "" {
		t.Error("expected version in response")
	}
}

// --- Dashboard auth ---

func TestCreateProject_NoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "test"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreateProject_WrongPassword(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "test"}, basicAuth("admin", "wrong"))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreateProject_Success(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "myproject"}, basicAuth("admin", "secret"))
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["name"] != "myproject" {
		t.Errorf("expected name myproject, got %v", resp["name"])
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Errorf("expected non-empty token in response, got %v", resp["token"])
	}
}

func TestCreateProject_InvalidName(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	cases := []string{"../evil", "foo/bar", ".hidden", "", "a b"}
	for _, name := range cases {
		w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": name}, auth)
		if w.Code != http.StatusBadRequest {
			t.Errorf("name %q: expected 400, got %d", name, w.Code)
		}
	}
}

func TestCreateProject_Duplicate(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "dup"}, auth)
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "dup"}, auth)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestListProjects(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "p1"}, auth)
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "p2"}, auth)

	w := doRequest(t, srv, "GET", "/api/v1/projects", nil, auth)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 projects, got %d", len(resp))
	}
	names := make(map[string]bool)
	for _, item := range resp {
		name, _ := item["name"].(string)
		names[name] = true
		if _, ok := item["feature_count"]; !ok {
			t.Errorf("project %q: expected feature_count in response", name)
		}
		if _, present := item["token"]; present {
			t.Errorf("project %q: token must not be exposed in list response", name)
		}
	}
	if !names["p1"] || !names["p2"] {
		t.Errorf("expected projects p1 and p2, got %v", names)
	}
}

func TestListProjects_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/projects", nil, basicAuth("admin", "secret"))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty list, got %d items", len(resp))
	}
}

func TestGetProject_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/projects/nope", nil, basicAuth("admin", "secret"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetProject_Found(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "findme"}, auth)
	w := doRequest(t, srv, "GET", "/api/v1/projects/findme", nil, auth)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["name"] != "findme" {
		t.Errorf("expected name findme, got %v", resp["name"])
	}
	if _, ok := resp["feature_count"]; !ok {
		t.Error("expected feature_count in response")
	}
	if _, present := resp["token"]; present {
		t.Error("token must not be exposed in GET /projects/{name} response")
	}
}

func TestDeleteProject(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "todelete"}, auth)
	w := doRequest(t, srv, "DELETE", "/api/v1/projects/todelete", nil, auth)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	// Verify it's gone
	w2 := doRequest(t, srv, "GET", "/api/v1/projects/todelete", nil, auth)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "DELETE", "/api/v1/projects/nope", nil, basicAuth("admin", "secret"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Token auth ---

func TestGetOwnProject_NoToken(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/project", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetOwnProject_InvalidToken(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/project", nil, bearerAuth("badtoken"))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetOwnProject_ValidToken(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")

	// Create project and get token
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "tokenproject"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	token, ok := created["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected non-empty token in create response, got %v", created["token"])
	}

	// Use token to get own project
	w2 := doRequest(t, srv, "GET", "/api/v1/project", nil, bearerAuth(token))
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode /project response: %v", err)
	}
	if resp["name"] != "tokenproject" {
		t.Errorf("expected name tokenproject, got %v", resp["name"])
	}
	if _, ok := resp["feature_count"]; !ok {
		t.Error("expected feature_count in response")
	}
}
