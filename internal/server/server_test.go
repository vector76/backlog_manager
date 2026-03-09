package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vector76/backlog_manager/internal/config"
	"github.com/vector76/backlog_manager/internal/model"
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
	srv, _ := server.New(cfg, st)
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

// --- Feature CRUD (dashboard) ---

func TestCreateFeature_Success(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)

	w := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{
		"name":        "My Feature",
		"description": "# Feature\nSome description.",
	}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["name"] != "My Feature" {
		t.Errorf("expected name 'My Feature', got %v", resp["name"])
	}
	if resp["status"] != "draft" {
		t.Errorf("expected status draft, got %v", resp["status"])
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Error("expected non-empty id")
	}
}

func TestCreateFeature_ProjectNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	w := doRequest(t, srv, "POST", "/api/v1/projects/nope/features", map[string]any{
		"name": "f",
	}, auth)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCreateFeature_MissingName(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	w := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{}, auth)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateFeature_DirectToBead(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)

	w := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{
		"name":          "Direct Feature",
		"description":   "Goes straight to ready.",
		"direct_to_bead": true,
	}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ready_to_generate" {
		t.Errorf("expected status ready_to_generate, got %v", resp["status"])
	}
}

func TestCreateFeature_DirectToBeadWithDependency(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)

	w := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{
		"name":           "Dependent Feature",
		"description":    "Waits for another.",
		"direct_to_bead":  true,
		"generate_after": "some-other-id",
	}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "waiting" {
		t.Errorf("expected status waiting, got %v", resp["status"])
	}
	if resp["generate_after"] != "some-other-id" {
		t.Errorf("expected generate_after some-other-id, got %v", resp["generate_after"])
	}
}

func TestListProjectFeatures_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)

	w := doRequest(t, srv, "GET", "/api/v1/projects/proj/features", nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty list, got %d", len(resp))
	}
}

func TestListProjectFeatures_WithStatusFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{"name": "f1"}, auth)
	doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{"name": "f2"}, auth)

	// Filter by draft — both should match
	w := doRequest(t, srv, "GET", "/api/v1/projects/proj/features?status=draft", nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 draft features, got %d", len(resp))
	}

	// Filter by awaiting_client — none should match
	w2 := doRequest(t, srv, "GET", "/api/v1/projects/proj/features?status=awaiting_client", nil, auth)
	var resp2 []map[string]any
	json.NewDecoder(w2.Body).Decode(&resp2)
	if len(resp2) != 0 {
		t.Errorf("expected 0 awaiting_client features, got %d", len(resp2))
	}
}

func TestListProjectFeatures_MultiStatusFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{"name": "f1"}, auth)

	w := doRequest(t, srv, "GET", "/api/v1/projects/proj/features?status=draft,abandoned", nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Errorf("expected 1 feature, got %d", len(resp))
	}
}

func TestListProjectFeatures_InvalidStatusFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)

	w := doRequest(t, srv, "GET", "/api/v1/projects/proj/features?status=notastatus", nil, auth)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid status, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClientListFeatures_InvalidStatusFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	token := createProjectAndToken(t, srv, "proj2")

	w := doRequest(t, srv, "GET", "/api/v1/features?status=bogus", nil, bearerAuth(token))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid status, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetProjectFeature_Detail(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	cw := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{
		"name":        "feat",
		"description": "my desc",
	}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	w := doRequest(t, srv, "GET", "/api/v1/projects/proj/features/"+id, nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["id"] != id {
		t.Errorf("expected id %q, got %v", id, resp["id"])
	}
	if resp["initial_description"] != "my desc" {
		t.Errorf("expected initial_description 'my desc', got %v", resp["initial_description"])
	}
}

func TestGetProjectFeature_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	w := doRequest(t, srv, "GET", "/api/v1/projects/proj/features/ft-nope", nil, auth)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateFeature_Name(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	cw := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{"name": "old"}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	w := doRequest(t, srv, "PATCH", "/api/v1/projects/proj/features/"+id, map[string]any{"name": "new name"}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["name"] != "new name" {
		t.Errorf("expected name 'new name', got %v", resp["name"])
	}
}

func TestUpdateFeature_EmptyNameRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	cw := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{"name": "orig"}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	w := doRequest(t, srv, "PATCH", "/api/v1/projects/proj/features/"+id, map[string]any{"name": ""}, auth)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateFeature_EmptyNameWithDescriptionLeavesDescriptionUnchanged(t *testing.T) {
	// Sending both a valid description and an empty name should fail atomically —
	// the description must not be written when name validation fails.
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	cw := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{
		"name":        "orig",
		"description": "original desc",
	}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	w := doRequest(t, srv, "PATCH", "/api/v1/projects/proj/features/"+id,
		map[string]any{"name": "", "description": "should not be saved"}, auth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	// Description must be unchanged
	dw := doRequest(t, srv, "GET", "/api/v1/projects/proj/features/"+id, nil, auth)
	var detail map[string]any
	json.NewDecoder(dw.Body).Decode(&detail)
	if detail["initial_description"] != "original desc" {
		t.Errorf("description was mutated despite 400 response: got %v", detail["initial_description"])
	}
}

func TestUpdateFeature_DescriptionInDraft(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	cw := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{
		"name":        "feat",
		"description": "old desc",
	}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	w := doRequest(t, srv, "PATCH", "/api/v1/projects/proj/features/"+id, map[string]any{"description": "new desc"}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify description updated via GET
	dw := doRequest(t, srv, "GET", "/api/v1/projects/proj/features/"+id, nil, auth)
	var detail map[string]any
	json.NewDecoder(dw.Body).Decode(&detail)
	if detail["initial_description"] != "new desc" {
		t.Errorf("expected updated description, got %v", detail["initial_description"])
	}
}

func TestUpdateFeature_DescriptionRejectedWhenNotDraft(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	cw := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{
		"name":        "feat",
		"description": "orig",
	}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	// Transition to awaiting_client so it's no longer draft
	if err := st.TransitionStatus("proj", id, model.StatusAwaitingClient); err != nil {
		t.Fatalf("transition: %v", err)
	}

	w := doRequest(t, srv, "PATCH", "/api/v1/projects/proj/features/"+id, map[string]any{"description": "new"}, auth)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAbandonFeature(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	cw := doRequest(t, srv, "POST", "/api/v1/projects/proj/features", map[string]any{"name": "feat"}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	w := doRequest(t, srv, "DELETE", "/api/v1/projects/proj/features/"+id, nil, auth)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify status is abandoned
	dw := doRequest(t, srv, "GET", "/api/v1/projects/proj/features/"+id, nil, auth)
	var detail map[string]any
	json.NewDecoder(dw.Body).Decode(&detail)
	if detail["status"] != "abandoned" {
		t.Errorf("expected status abandoned, got %v", detail["status"])
	}
}

func TestAbandonFeature_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "proj"}, auth)
	w := doRequest(t, srv, "DELETE", "/api/v1/projects/proj/features/ft-nope", nil, auth)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Client feature endpoints (token auth) ---

func createProjectAndToken(t *testing.T, srv *http.Server, projectName string) string {
	t.Helper()
	auth := basicAuth("admin", "secret")
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": projectName}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	return resp["token"].(string)
}

func TestClientListFeatures(t *testing.T) {
	srv, _ := newTestServer(t)
	token := createProjectAndToken(t, srv, "clientproj")
	auth := basicAuth("admin", "secret")

	doRequest(t, srv, "POST", "/api/v1/projects/clientproj/features", map[string]any{"name": "f1"}, auth)
	doRequest(t, srv, "POST", "/api/v1/projects/clientproj/features", map[string]any{"name": "f2"}, auth)

	w := doRequest(t, srv, "GET", "/api/v1/features", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 2 {
		t.Errorf("expected 2 features, got %d", len(resp))
	}
}

func TestClientListFeatures_NoToken(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/features", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestClientGetFeatureDetail(t *testing.T) {
	srv, _ := newTestServer(t)
	token := createProjectAndToken(t, srv, "clientproj")
	auth := basicAuth("admin", "secret")

	cw := doRequest(t, srv, "POST", "/api/v1/projects/clientproj/features", map[string]any{
		"name":        "feat",
		"description": "hello",
	}, auth)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	w := doRequest(t, srv, "GET", "/api/v1/features/"+id, nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["id"] != id {
		t.Errorf("expected id %q, got %v", id, resp["id"])
	}
	if resp["initial_description"] != "hello" {
		t.Errorf("expected initial_description 'hello', got %v", resp["initial_description"])
	}
}

func TestClientGetFeatureDetail_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	token := createProjectAndToken(t, srv, "clientproj")
	w := doRequest(t, srv, "GET", "/api/v1/features/ft-nope", nil, bearerAuth(token))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestClientListFeatures_StatusFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	token := createProjectAndToken(t, srv, "clientproj")
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects/clientproj/features", map[string]any{"name": "f1"}, auth)

	w := doRequest(t, srv, "GET", "/api/v1/features?status=draft", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 1 {
		t.Errorf("expected 1 feature, got %d", len(resp))
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

// --- Dialog state machine endpoint tests ---

// setupFeatureWithStatus creates a project and feature, then transitions the feature
// to the desired status using the store directly.
func setupFeatureWithStatus(t *testing.T, srv *http.Server, st *store.Store, status model.FeatureStatus) (projectName, featureID string) {
	t.Helper()
	auth := basicAuth("admin", "secret")
	projectName = "dialogproj"

	// Create project
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": projectName}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}

	// Create feature
	w = doRequest(t, srv, "POST", "/api/v1/projects/"+projectName+"/features",
		map[string]any{"name": "feat1", "description": "desc"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d %s", w.Code, w.Body.String())
	}
	var feat map[string]any
	if err := json.NewDecoder(w.Body).Decode(&feat); err != nil {
		t.Fatal(err)
	}
	featureID = feat["id"].(string)

	// Transition through statuses to reach the desired state
	transitions := []model.FeatureStatus{}
	switch status {
	case model.StatusDraft:
		// no transitions needed
	case model.StatusAwaitingClient:
		transitions = []model.FeatureStatus{model.StatusAwaitingClient}
	case model.StatusAwaitingHuman:
		transitions = []model.FeatureStatus{model.StatusAwaitingClient, model.StatusAwaitingHuman}
	case model.StatusFullySpecified:
		transitions = []model.FeatureStatus{model.StatusAwaitingClient, model.StatusFullySpecified}
	}

	for _, s := range transitions {
		if err := st.TransitionStatus(projectName, featureID, s); err != nil {
			t.Fatalf("transition to %v: %v", s, err)
		}
	}

	// For awaiting_human, set CurrentIteration=1 so RespondToDialog has a round to respond to.
	// WriteClientRound doesn't check status, so it can be called in any state.
	if status == model.StatusAwaitingHuman {
		if err := st.WriteClientRound(projectName, featureID, 1, "desc v1", "questions"); err != nil {
			t.Fatalf("write client round: %v", err)
		}
	}

	return projectName, featureID
}

func TestHandleStartDialog(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/start-dialog"

	w := doRequest(t, srv, "POST", path, nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "awaiting_client" {
		t.Errorf("expected status awaiting_client, got %v", resp["status"])
	}
}

func TestHandleStartDialog_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingClient)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/start-dialog"

	w := doRequest(t, srv, "POST", path, nil, auth)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartDialog_NoAuth(t *testing.T) {
	srv, st := newTestServer(t)
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/start-dialog"
	w := doRequest(t, srv, "POST", path, nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleRespondToDialog(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingHuman)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/respond"

	w := doRequest(t, srv, "POST", path, map[string]any{"response": "my answer", "final": false}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "awaiting_client" {
		t.Errorf("expected status awaiting_client, got %v", resp["status"])
	}
}

func TestHandleRespondToDialog_Final(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingHuman)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/respond"

	w := doRequest(t, srv, "POST", path, map[string]any{"response": "final answer", "final": true}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify is_final was set on the correct iteration (round 1, since setupFeatureWithStatus uses WriteClientRound(1,...))
	f, err := st.GetFeature(projectName, featureID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range f.Iterations {
		if it.Round == 1 && it.IsFinal {
			found = true
		}
	}
	if !found {
		t.Errorf("expected is_final=true for round 1 after final respond; got Iterations=%v", f.Iterations)
	}
}

func TestHandleRespondToDialog_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingClient)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/respond"

	w := doRequest(t, srv, "POST", path, map[string]any{"response": "answer", "final": false}, auth)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRespondToDialog_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")

	w := doRequest(t, srv, "POST", "/api/v1/projects/noproject/features/nofeat/respond",
		map[string]any{"response": "ans", "final": false}, auth)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReopenDialog(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusFullySpecified)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/reopen"

	w := doRequest(t, srv, "POST", path, map[string]any{"message": "please add feature X"}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "awaiting_client" {
		t.Errorf("expected status awaiting_client, got %v", resp["status"])
	}
}

func TestHandleReopenDialog_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingClient)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/reopen"

	w := doRequest(t, srv, "POST", path, map[string]any{"message": "message"}, auth)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReopenDialog_NoAuth(t *testing.T) {
	srv, st := newTestServer(t)
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusFullySpecified)
	path := "/api/v1/projects/" + projectName + "/features/" + featureID + "/reopen"
	w := doRequest(t, srv, "POST", path, map[string]any{"message": "msg"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- Client poll endpoint tests ---

// getProjectToken creates a project via dashboard and returns its token.
func getProjectToken(t *testing.T, srv *http.Server, projectName string) string {
	t.Helper()
	auth := basicAuth("admin", "secret")
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": projectName}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	return resp["token"].(string)
}

// tokenForProject returns the bearer token for an existing project from the store.
func tokenForProject(t *testing.T, st *store.Store, projectName string) string {
	t.Helper()
	p, err := st.GetProject(projectName)
	if err != nil {
		t.Fatalf("get project %q: %v", projectName, err)
	}
	return p.Token
}

func TestHandlePoll_NoWork_Returns204(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getProjectToken(t, srv, "pollproj1")

	w := doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(token))
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 when no work, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePoll_WithWork_Returns200(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	// Create project and get token.
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "pollproj2"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d", w.Code)
	}
	var projResp map[string]any
	json.NewDecoder(w.Body).Decode(&projResp)
	token := projResp["token"].(string)

	// Create feature and start dialog → awaiting_client.
	w = doRequest(t, srv, "POST", "/api/v1/projects/pollproj2/features",
		map[string]any{"name": "feat", "description": "desc"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d", w.Code)
	}
	var feat map[string]any
	json.NewDecoder(w.Body).Decode(&feat)
	featureID := feat["id"].(string)

	if err := st.StartDialog("pollproj2", featureID); err != nil {
		t.Fatal(err)
	}

	// Poll should return immediately with work.
	w = doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with work, got %d: %s", w.Code, w.Body.String())
	}
	var pollResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&pollResp); err != nil {
		t.Fatal(err)
	}
	if pollResp["action"] != "dialog_step" {
		t.Errorf("expected action=dialog_step, got %v", pollResp["action"])
	}
	if pollResp["feature_id"] != featureID {
		t.Errorf("expected feature_id=%s, got %v", featureID, pollResp["feature_id"])
	}
	if pollResp["feature_name"] != "feat" {
		t.Errorf("expected feature_name=feat, got %v", pollResp["feature_name"])
	}
}

func TestHandlePoll_NoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/poll", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandlePoll_RecordsConnectivity(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getProjectToken(t, srv, "connproj")

	// Before poll: no connectivity.
	w := doRequest(t, srv, "GET", "/api/v1/project", nil, bearerAuth(token))
	var proj map[string]any
	json.NewDecoder(w.Body).Decode(&proj)
	if proj["connectivity"] != nil && proj["connectivity"] != "" {
		t.Errorf("expected no connectivity before poll, got %v", proj["connectivity"])
	}

	// Poll (timeout=1 to avoid blocking).
	doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(token))

	// After poll: "Connected (<1 min)".
	w = doRequest(t, srv, "GET", "/api/v1/project", nil, bearerAuth(token))
	json.NewDecoder(w.Body).Decode(&proj)
	if proj["connectivity"] != "Connected (<1 min)" {
		t.Errorf("expected connectivity=Connected (<1 min) after poll, got %v", proj["connectivity"])
	}
}

func TestHandlePoll_DirectToBead_InResponse(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")

	// Create project and get token.
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "dtbpollproj"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d", w.Code)
	}
	var projResp map[string]any
	json.NewDecoder(w.Body).Decode(&projResp)
	token := projResp["token"].(string)

	// Create feature with direct_to_bead=true → goes to ready_to_generate immediately.
	w = doRequest(t, srv, "POST", "/api/v1/projects/dtbpollproj/features",
		map[string]any{"name": "dtb-feat", "direct_to_bead": true}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d", w.Code)
	}

	// Poll should return with action=generate and direct_to_bead=true.
	w = doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var pollResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&pollResp); err != nil {
		t.Fatal(err)
	}
	if pollResp["action"] != "generate" {
		t.Errorf("expected action=generate, got %v", pollResp["action"])
	}
	if pollResp["direct_to_bead"] != true {
		t.Errorf("expected direct_to_bead=true, got %v", pollResp["direct_to_bead"])
	}
}

func TestHandlePoll_DialogStep_NoDirectToBead(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	// Create project and get token.
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "nodtbpollproj"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d", w.Code)
	}
	var projResp map[string]any
	json.NewDecoder(w.Body).Decode(&projResp)
	token := projResp["token"].(string)

	// Create feature (no direct_to_bead) and put it in awaiting_client.
	w = doRequest(t, srv, "POST", "/api/v1/projects/nodtbpollproj/features",
		map[string]any{"name": "feat"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d", w.Code)
	}
	var feat map[string]any
	json.NewDecoder(w.Body).Decode(&feat)
	featureID := feat["id"].(string)

	if err := st.StartDialog("nodtbpollproj", featureID); err != nil {
		t.Fatal(err)
	}

	// Poll should return dialog_step with no direct_to_bead field.
	w = doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var pollResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&pollResp); err != nil {
		t.Fatal(err)
	}
	if pollResp["action"] != "dialog_step" {
		t.Errorf("expected action=dialog_step, got %v", pollResp["action"])
	}
	if _, present := pollResp["direct_to_bead"]; present {
		t.Errorf("expected direct_to_bead absent for dialog_step, got %v", pollResp["direct_to_bead"])
	}
}

func TestCreateFeature_DirectToBead_InResponse(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "dtbresp"}, auth)

	w := doRequest(t, srv, "POST", "/api/v1/projects/dtbresp/features", map[string]any{
		"name":           "DTB Feature",
		"direct_to_bead": true,
	}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["direct_to_bead"] != true {
		t.Errorf("expected direct_to_bead=true in response, got %v", resp["direct_to_bead"])
	}
}

// --- Pending endpoint tests ---

func TestHandleGetPending_FirstRound(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)
	token := tokenForProject(t, st, projectName)

	// Start dialog → awaiting_client, iteration=0.
	w := doRequest(t, srv, "POST",
		"/api/v1/projects/"+projectName+"/features/"+featureID+"/start-dialog",
		nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("start-dialog: %d", w.Code)
	}

	w = doRequest(t, srv, "GET",
		"/api/v1/features/"+featureID+"/pending", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("pending: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["iteration"] != float64(0) {
		t.Errorf("expected iteration=0, got %v", resp["iteration"])
	}
	if resp["feature_description"] == "" {
		t.Error("expected non-empty feature_description")
	}
	if resp["questions"] != "" && resp["questions"] != nil {
		t.Errorf("expected empty questions, got %v", resp["questions"])
	}
	if resp["user_response"] != "" && resp["user_response"] != nil {
		t.Errorf("expected empty user_response, got %v", resp["user_response"])
	}
}

func TestHandleGetPending_SubsequentRound(t *testing.T) {
	srv, st := newTestServer(t)
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingHuman)
	// setupFeatureWithStatus with AwaitingHuman calls WriteClientRound(1, "desc v1", "questions")
	token := tokenForProject(t, st, projectName)

	// Respond to make feature awaiting_client again.
	if err := st.RespondToDialog(projectName, featureID, "my answer", false); err != nil {
		t.Fatal(err)
	}

	w := doRequest(t, srv, "GET",
		"/api/v1/features/"+featureID+"/pending", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("pending: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["iteration"] != float64(1) {
		t.Errorf("expected iteration=1, got %v", resp["iteration"])
	}
	if resp["feature_description"] == "" {
		t.Error("expected non-empty feature_description")
	}
	if resp["questions"] == "" {
		t.Error("expected non-empty questions")
	}
	if resp["user_response"] == "" {
		t.Error("expected non-empty user_response")
	}
}

func TestHandleGetPending_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getProjectToken(t, srv, "pendingproj")

	w := doRequest(t, srv, "GET", "/api/v1/features/ft-notexist/pending", nil, bearerAuth(token))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetPending_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)
	token := tokenForProject(t, st, projectName)

	w := doRequest(t, srv, "GET",
		"/api/v1/features/"+featureID+"/pending", nil, bearerAuth(token))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for non-actionable feature, got %d", w.Code)
	}
}

func TestHandleGetPending_IsFinalFlag(t *testing.T) {
	srv, st := newTestServer(t)
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingHuman)
	token := tokenForProject(t, st, projectName)

	// Respond with final=true → feature becomes awaiting_client.
	if err := st.RespondToDialog(projectName, featureID, "final answer", true); err != nil {
		t.Fatal(err)
	}

	w := doRequest(t, srv, "GET",
		"/api/v1/features/"+featureID+"/pending", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("pending: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["is_final"] != true {
		t.Errorf("expected is_final=true after final response, got %v", resp["is_final"])
	}
}

func TestHandleGetPending_IsFinalFalseWhenNotFinal(t *testing.T) {
	srv, st := newTestServer(t)
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingHuman)
	token := tokenForProject(t, st, projectName)

	// Respond with final=false → feature becomes awaiting_client.
	if err := st.RespondToDialog(projectName, featureID, "my answer", false); err != nil {
		t.Fatal(err)
	}

	w := doRequest(t, srv, "GET",
		"/api/v1/features/"+featureID+"/pending", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("pending: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["is_final"] == true {
		t.Errorf("expected is_final=false for non-final response, got %v", resp["is_final"])
	}
}

func TestHandleGetPending_AfterReopen(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusFullySpecified)
	token := tokenForProject(t, st, projectName)

	// Reopen the feature.
	w := doRequest(t, srv, "POST",
		"/api/v1/projects/"+projectName+"/features/"+featureID+"/reopen",
		map[string]string{"message": "Actually, one more thing."}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("reopen: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = doRequest(t, srv, "GET",
		"/api/v1/features/"+featureID+"/pending", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("pending after reopen: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["feature_description"] == "" {
		t.Error("expected non-empty feature_description after reopen")
	}
	if resp["user_response"] == "" {
		t.Error("expected reopen message in user_response")
	}
	if resp["questions"] != "" && resp["questions"] != nil {
		t.Errorf("expected empty questions in reopen case, got %v", resp["questions"])
	}
	if resp["is_final"] == true {
		t.Errorf("expected is_final=false in reopen case, got %v", resp["is_final"])
	}
}

func TestHandleGetPending_NoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "GET", "/api/v1/features/ft-abc/pending", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- Submit-dialog endpoint tests ---

func TestHandleSubmitDialog_ToAwaitingHuman(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)
	token := tokenForProject(t, st, projectName)

	// Start dialog.
	doRequest(t, srv, "POST",
		"/api/v1/projects/"+projectName+"/features/"+featureID+"/start-dialog",
		nil, auth)

	// Submit with questions → should go to awaiting_human.
	w := doRequest(t, srv, "POST",
		"/api/v1/features/"+featureID+"/submit-dialog",
		map[string]string{"updated_description": "new desc", "questions": "Any questions?"},
		bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("submit-dialog: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "awaiting_human" {
		t.Errorf("expected awaiting_human, got %v", resp["status"])
	}
}

func TestHandleSubmitDialog_ToFullySpecified(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingHuman)
	token := tokenForProject(t, st, projectName)

	// Respond with final=true → awaiting_client.
	doRequest(t, srv, "POST",
		"/api/v1/projects/"+projectName+"/features/"+featureID+"/respond",
		map[string]any{"response": "final answer", "final": true}, auth)

	// Submit with no questions + is_final → should go to fully_specified.
	w := doRequest(t, srv, "POST",
		"/api/v1/features/"+featureID+"/submit-dialog",
		map[string]string{"updated_description": "final desc", "questions": ""},
		bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("submit-dialog: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "fully_specified" {
		t.Errorf("expected fully_specified, got %v", resp["status"])
	}
}

func TestHandleSubmitDialog_WithQuestionsAfterFinalResponse(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusAwaitingHuman)
	token := tokenForProject(t, st, projectName)

	// Respond with final=true → awaiting_client.
	doRequest(t, srv, "POST",
		"/api/v1/projects/"+projectName+"/features/"+featureID+"/respond",
		map[string]any{"response": "final answer", "final": true}, auth)

	// Submit WITH questions despite final response → should stay awaiting_human, not fully_specified.
	w := doRequest(t, srv, "POST",
		"/api/v1/features/"+featureID+"/submit-dialog",
		map[string]string{"updated_description": "desc", "questions": "Still have questions?"},
		bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("submit-dialog: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "awaiting_human" {
		t.Errorf("expected awaiting_human when questions submitted despite final response, got %v", resp["status"])
	}
}

func TestHandleSubmitDialog_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	projectName, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)
	token := tokenForProject(t, st, projectName)

	w := doRequest(t, srv, "POST",
		"/api/v1/features/"+featureID+"/submit-dialog",
		map[string]string{"updated_description": "desc", "questions": "q"},
		bearerAuth(token))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleSubmitDialog_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	token := getProjectToken(t, srv, "submitproj")

	w := doRequest(t, srv, "POST",
		"/api/v1/features/ft-notexist/submit-dialog",
		map[string]string{"updated_description": "desc", "questions": "q"},
		bearerAuth(token))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSubmitDialog_NoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "POST", "/api/v1/features/ft-abc/submit-dialog",
		map[string]string{"updated_description": "d", "questions": "q"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- Generation pipeline handlers ---

func setupFeatureAtFullySpecified(t *testing.T, srv *http.Server, st *store.Store, projectName string) string {
	t.Helper()
	auth := basicAuth("admin", "secret")

	// Create project
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": projectName}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d", w.Code)
	}

	// Create feature
	w = doRequest(t, srv, "POST", "/api/v1/projects/"+projectName+"/features",
		map[string]any{"name": "feat", "description": "desc"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d", w.Code)
	}
	var feat map[string]any
	if err := json.NewDecoder(w.Body).Decode(&feat); err != nil {
		t.Fatal(err)
	}
	featureID := feat["id"].(string)

	// Advance to fully_specified
	for _, s := range []model.FeatureStatus{model.StatusAwaitingClient, model.StatusFullySpecified} {
		if err := st.TransitionStatus(projectName, featureID, s); err != nil {
			t.Fatalf("transition to %v: %v", s, err)
		}
	}
	return featureID
}

func TestHandleGenerateNow_Success(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	featureID := setupFeatureAtFullySpecified(t, srv, st, "gennow")

	path := "/api/v1/projects/gennow/features/" + featureID + "/generate-now"
	w := doRequest(t, srv, "POST", path, nil, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ready_to_generate" {
		t.Errorf("expected status ready_to_generate, got %v", resp["status"])
	}
}

func TestHandleGenerateNow_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	_, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)

	path := "/api/v1/projects/dialogproj/features/" + featureID + "/generate-now"
	w := doRequest(t, srv, "POST", path, nil, auth)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleGenerateAfter_Success(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	featureID := setupFeatureAtFullySpecified(t, srv, st, "genafter")

	// Create a second feature to depend on
	w := doRequest(t, srv, "POST", "/api/v1/projects/genafter/features",
		map[string]any{"name": "other", "description": "desc"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create other feature: %d", w.Code)
	}
	var other map[string]any
	if err := json.NewDecoder(w.Body).Decode(&other); err != nil {
		t.Fatal(err)
	}
	otherID := other["id"].(string)

	path := "/api/v1/projects/genafter/features/" + featureID + "/generate-after"
	w = doRequest(t, srv, "POST", path, map[string]string{"after_feature_id": otherID}, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "waiting" {
		t.Errorf("expected status waiting, got %v", resp["status"])
	}
	if resp["generate_after"] != otherID {
		t.Errorf("expected generate_after %q, got %v", otherID, resp["generate_after"])
	}
}

func TestHandleGenerateAfter_MissingField(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")
	featureID := setupFeatureAtFullySpecified(t, srv, st, "genafter2")

	path := "/api/v1/projects/genafter2/features/" + featureID + "/generate-after"
	w := doRequest(t, srv, "POST", path, map[string]string{}, auth)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleStartGenerate_Success(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "startgen")
	if err := st.TransitionStatus("startgen", featureID, model.StatusReadyToGenerate); err != nil {
		t.Fatalf("transition: %v", err)
	}
	token := tokenForProject(t, st, "startgen")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/start-generate", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "generating" {
		t.Errorf("expected status generating, got %v", resp["status"])
	}
}

func TestHandleStartGenerate_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "startgen2")
	token := tokenForProject(t, st, "startgen2")

	// Feature is in fully_specified, not ready_to_generate
	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/start-generate", nil, bearerAuth(token))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func setupFeatureAtGenerating(t *testing.T, srv *http.Server, st *store.Store, projectName string) string {
	t.Helper()
	featureID := setupFeatureAtFullySpecified(t, srv, st, projectName)
	for _, s := range []model.FeatureStatus{model.StatusReadyToGenerate, model.StatusGenerating} {
		if err := st.TransitionStatus(projectName, featureID, s); err != nil {
			t.Fatalf("transition to %v: %v", s, err)
		}
	}
	return featureID
}

func TestHandleRegisterBead_Success(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtGenerating(t, srv, st, "regbead")
	token := tokenForProject(t, st, "regbead")

	// Register first bead — should stay generating.
	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-bead",
		map[string]any{"bead_id": "bd-aaa1"}, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "generating" {
		t.Errorf("expected status still generating after register-bead, got %v", resp["status"])
	}
	beadIDs, _ := resp["bead_ids"].([]any)
	if len(beadIDs) != 1 {
		t.Errorf("expected 1 bead_id, got %v", resp["bead_ids"])
	}

	// Register second bead — should accumulate.
	w = doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-bead",
		map[string]any{"bead_id": "bd-bbb2"}, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	json.NewDecoder(w.Body).Decode(&resp)
	beadIDs, _ = resp["bead_ids"].([]any)
	if len(beadIDs) != 2 {
		t.Errorf("expected 2 bead_ids after second register-bead, got %v", resp["bead_ids"])
	}
}

func TestHandleRegisterBead_MissingBeadID(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtGenerating(t, srv, st, "regbead2")
	token := tokenForProject(t, st, "regbead2")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-bead",
		map[string]any{"bead_id": ""}, bearerAuth(token))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRegisterBead_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "regbead3")
	token := tokenForProject(t, st, "regbead3")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-bead",
		map[string]any{"bead_id": "bd-111"}, bearerAuth(token))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
	f, _ := st.GetFeature("regbead3", featureID)
	if len(f.BeadIDs) != 0 {
		t.Errorf("BeadIDs must not be persisted when status is wrong, got %v", f.BeadIDs)
	}
}

func TestHandleBeadsDone_Success(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtGenerating(t, srv, st, "beadsdone")
	token := tokenForProject(t, st, "beadsdone")

	// Register a bead then finalize.
	doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-bead",
		map[string]any{"bead_id": "bd-aaa1"}, bearerAuth(token))

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/beads-done", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "beads_created" {
		t.Errorf("expected beads_created, got %v", resp["status"])
	}
}

func TestHandleBeadsDone_EmptyBeadList(t *testing.T) {
	// beads-done with zero registered beads is valid.
	srv, st := newTestServer(t)
	featureID := setupFeatureAtGenerating(t, srv, st, "beadsdone2")
	token := tokenForProject(t, st, "beadsdone2")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/beads-done", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "beads_created" {
		t.Errorf("expected beads_created, got %v", resp["status"])
	}
}

func TestHandleBeadsDone_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "beadsdone3")
	token := tokenForProject(t, st, "beadsdone3")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/beads-done", nil, bearerAuth(token))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleGenerateAfter_WrongStatus_DoesNotPersistGenerateAfter(t *testing.T) {
	// Feature is in draft (not fully_specified) — GenerateAfter must not be persisted.
	srv, st := newTestServer(t)
	_, featureID := setupFeatureWithStatus(t, srv, st, model.StatusDraft)
	auth := basicAuth("admin", "secret")

	path := "/api/v1/projects/dialogproj/features/" + featureID + "/generate-after"
	w := doRequest(t, srv, "POST", path, map[string]string{"after_feature_id": "ft-other"}, auth)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
	f, err := st.GetFeature("dialogproj", featureID)
	if err != nil {
		t.Fatal(err)
	}
	if f.GenerateAfter != "" {
		t.Errorf("GenerateAfter must not be persisted when status is wrong, got %q", f.GenerateAfter)
	}
}

func TestHandleRegisterArtifact_Success(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "regartifact")
	token := tokenForProject(t, st, "regartifact")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-artifact",
		map[string]string{"type": "plan", "content": "# Plan\nDo stuff."}, bearerAuth(token))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRegisterArtifact_InvalidType(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "regartifact2")
	token := tokenForProject(t, st, "regartifact2")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/register-artifact",
		map[string]string{"type": "invalid", "content": "stuff"}, bearerAuth(token))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRegisterArtifact_NoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := doRequest(t, srv, "POST", "/api/v1/features/ft-abc/register-artifact",
		map[string]string{"type": "plan", "content": "stuff"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleRegisterArtifact_UpdatesLastSeen(t *testing.T) {
	srv, st := newTestServer(t)
	_, err := st.CreateProject("poll-project", "tok-poll")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	f, err := st.CreateFeature("poll-project", "Poll Feature", "desc", false, "")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}

	w := doRequest(t, srv, "POST", "/api/v1/features/"+f.ID+"/register-artifact",
		map[string]any{"type": "plan", "content": "# Plan"}, bearerAuth("tok-poll"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	ts, ok := st.GetLastPollTime("poll-project")
	if !ok {
		t.Fatal("expected poll time to be recorded, but ok == false")
	}
	if ts.IsZero() {
		t.Fatal("expected non-zero poll time")
	}
}

func TestHandleCompleteFeature_Success(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "complete1")
	for _, s := range []model.FeatureStatus{model.StatusReadyToGenerate, model.StatusGenerating, model.StatusBeadsCreated} {
		if err := st.TransitionStatus("complete1", featureID, s); err != nil {
			t.Fatalf("transition to %v: %v", s, err)
		}
	}
	token := tokenForProject(t, st, "complete1")

	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/complete", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" {
		t.Errorf("expected status done, got %v", resp["status"])
	}
}

func TestHandleCompleteFeature_DependencyResolution(t *testing.T) {
	srv, st := newTestServer(t)
	auth := basicAuth("admin", "secret")

	// Create project with two features
	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "depres"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d", w.Code)
	}

	w = doRequest(t, srv, "POST", "/api/v1/projects/depres/features",
		map[string]any{"name": "provider", "description": "desc"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create provider: %d", w.Code)
	}
	var provFeat map[string]any
	json.NewDecoder(w.Body).Decode(&provFeat)
	providerID := provFeat["id"].(string)

	w = doRequest(t, srv, "POST", "/api/v1/projects/depres/features",
		map[string]any{"name": "waiter", "description": "desc"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create waiter: %d", w.Code)
	}
	var waiterFeat map[string]any
	json.NewDecoder(w.Body).Decode(&waiterFeat)
	waiterID := waiterFeat["id"].(string)

	// Advance provider to beads_created
	for _, s := range []model.FeatureStatus{model.StatusAwaitingClient, model.StatusFullySpecified, model.StatusReadyToGenerate, model.StatusGenerating, model.StatusBeadsCreated} {
		if err := st.TransitionStatus("depres", providerID, s); err != nil {
			t.Fatalf("transition provider to %v: %v", s, err)
		}
	}

	// Set waiter to waiting with dependency on provider
	if err := st.TransitionStatus("depres", waiterID, model.StatusAwaitingClient); err != nil {
		t.Fatalf("transition waiter: %v", err)
	}
	if err := st.TransitionStatus("depres", waiterID, model.StatusFullySpecified); err != nil {
		t.Fatalf("transition waiter: %v", err)
	}
	waiter, err := st.GetFeature("depres", waiterID)
	if err != nil {
		t.Fatal(err)
	}
	waiter.GenerateAfter = providerID
	if err := st.UpdateFeature(waiter); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("depres", waiterID, model.StatusWaiting); err != nil {
		t.Fatalf("transition waiter to waiting: %v", err)
	}

	// Complete the provider feature
	token := tokenForProject(t, st, "depres")
	w = doRequest(t, srv, "POST", "/api/v1/features/"+providerID+"/complete", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify waiter is now ready_to_generate
	waiterUpdated, err := st.GetFeature("depres", waiterID)
	if err != nil {
		t.Fatal(err)
	}
	if waiterUpdated.Status != model.StatusReadyToGenerate {
		t.Errorf("expected waiter to be ready_to_generate after provider completes, got %v", waiterUpdated.Status)
	}
}

// --- direct_to_bead API tests ---

func TestHandleCreateFeature_DirectToBead(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")
	doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "dtbproj"}, auth)

	w := doRequest(t, srv, "POST", "/api/v1/projects/dtbproj/features", map[string]any{
		"name":           "DTB Feature",
		"description":    "Goes straight to bead.",
		"direct_to_bead": true,
	}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ready_to_generate" {
		t.Errorf("expected status ready_to_generate, got %v", resp["status"])
	}
	if resp["direct_to_bead"] != true {
		t.Errorf("expected direct_to_bead=true, got %v", resp["direct_to_bead"])
	}
}

func TestHandleCreateFeature_DirectToBead_ImmediatelyPollable(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")

	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "dtbpoll"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d", w.Code)
	}
	var projResp map[string]any
	json.NewDecoder(w.Body).Decode(&projResp)
	token := projResp["token"].(string)

	// Create a direct_to_bead feature — should immediately be in ready_to_generate.
	w = doRequest(t, srv, "POST", "/api/v1/projects/dtbpoll/features", map[string]any{
		"name":           "Pollable DTB",
		"direct_to_bead": true,
	}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d", w.Code)
	}

	// Poll should return action=generate immediately.
	w = doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from poll, got %d: %s", w.Code, w.Body.String())
	}
	var pollResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&pollResp); err != nil {
		t.Fatal(err)
	}
	if pollResp["action"] != "generate" {
		t.Errorf("expected action=generate, got %v", pollResp["action"])
	}
}

func TestHandleCreateFeature_NormalFeature_StillDraft(t *testing.T) {
	srv, _ := newTestServer(t)
	auth := basicAuth("admin", "secret")

	w := doRequest(t, srv, "POST", "/api/v1/projects", map[string]any{"name": "draftproj"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d", w.Code)
	}
	var projResp map[string]any
	json.NewDecoder(w.Body).Decode(&projResp)
	token := projResp["token"].(string)

	// Create a normal feature (no direct_to_bead).
	w = doRequest(t, srv, "POST", "/api/v1/projects/draftproj/features", map[string]any{
		"name":        "Normal Feature",
		"description": "No direct_to_bead.",
	}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "draft" {
		t.Errorf("expected status draft, got %v", resp["status"])
	}

	// Poll should return 204 (no actionable work) since the feature is still in draft.
	w = doRequest(t, srv, "GET", "/api/v1/poll?timeout=1", nil, bearerAuth(token))
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 (no work for draft feature), got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBeadsDone_WaitingDependentStaysWaiting(t *testing.T) {
	srv, st := newTestServer(t)

	// Create provider feature A and advance to generating.
	providerID := setupFeatureAtGenerating(t, srv, st, "bdwait")
	token := tokenForProject(t, st, "bdwait")

	// Create dependent feature B in the same project.
	auth := basicAuth("admin", "secret")
	w := doRequest(t, srv, "POST", "/api/v1/projects/bdwait/features",
		map[string]any{"name": "waiter", "description": "desc"}, auth)
	if w.Code != http.StatusCreated {
		t.Fatalf("create waiter: %d", w.Code)
	}
	var waiterFeat map[string]any
	json.NewDecoder(w.Body).Decode(&waiterFeat)
	waiterID := waiterFeat["id"].(string)

	// Advance B to fully_specified, set GenerateAfter = A, then transition to waiting.
	if err := st.TransitionStatus("bdwait", waiterID, model.StatusAwaitingClient); err != nil {
		t.Fatalf("transition waiter: %v", err)
	}
	if err := st.TransitionStatus("bdwait", waiterID, model.StatusFullySpecified); err != nil {
		t.Fatalf("transition waiter: %v", err)
	}
	wf, err := st.GetFeature("bdwait", waiterID)
	if err != nil {
		t.Fatal(err)
	}
	wf.GenerateAfter = providerID
	if err := st.UpdateFeature(wf); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("bdwait", waiterID, model.StatusWaiting); err != nil {
		t.Fatalf("transition waiter to waiting: %v", err)
	}

	// POST beads-done for A (transitions A from generating → beads_created).
	w = doRequest(t, srv, "POST", "/api/v1/features/"+providerID+"/beads-done", nil, bearerAuth(token))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// B must still be waiting — beads_created must NOT trigger dependency resolution.
	waiterUpdated, err := st.GetFeature("bdwait", waiterID)
	if err != nil {
		t.Fatal(err)
	}
	if waiterUpdated.Status != model.StatusWaiting {
		t.Errorf("expected waiter to remain waiting after provider beads-done, got %v", waiterUpdated.Status)
	}
}

func TestHandleCompleteFeature_WrongStatus(t *testing.T) {
	srv, st := newTestServer(t)
	featureID := setupFeatureAtFullySpecified(t, srv, st, "complete2")
	token := tokenForProject(t, st, "complete2")

	// Feature is in fully_specified, not beads_created
	w := doRequest(t, srv, "POST", "/api/v1/features/"+featureID+"/complete", nil, bearerAuth(token))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}
