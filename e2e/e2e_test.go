// Package e2e contains end-to-end tests for the backlog manager.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vector76/backlog_manager/internal/config"
	"github.com/vector76/backlog_manager/internal/server"
	"github.com/vector76/backlog_manager/internal/store"
)

// e2eServer bundles together a test HTTP server and its underlying store.
type e2eServer struct {
	srv   *httptest.Server
	st    *store.Store
	token string
}

func newE2EServer(t *testing.T) *e2eServer {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:              0,
		DataDir:           t.TempDir(),
		DashboardUser:     "admin",
		DashboardPassword: "secret",
	}
	httpSrv := server.New(cfg, st)
	ts := httptest.NewServer(httpSrv.Handler)
	t.Cleanup(ts.Close)
	return &e2eServer{srv: ts, st: st}
}

func (s *e2eServer) do(t *testing.T, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewBuffer(data)
	} else {
		buf = &bytes.Buffer{}
	}
	req, err := http.NewRequest(method, s.srv.URL+path, buf)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (s *e2eServer) dashboardAuth() map[string]string {
	req, _ := http.NewRequest("GET", "/", nil)
	req.SetBasicAuth("admin", "secret")
	return map[string]string{"Authorization": req.Header.Get("Authorization")}
}

func (s *e2eServer) bearerAuth() map[string]string {
	return map[string]string{"Authorization": "Bearer " + s.token}
}

func decode(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// setupProject creates a project via the dashboard API and sets s.token.
func (s *e2eServer) setupProject(t *testing.T, projectName string) {
	t.Helper()
	resp := s.do(t, "POST", "/api/v1/projects", map[string]string{"name": projectName}, s.dashboardAuth())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d", resp.StatusCode)
	}
	var result map[string]any
	decode(t, resp, &result)
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatal("create project: no token in response")
	}
	s.token = token
}

// createFeature creates a feature and returns its ID.
func (s *e2eServer) createFeature(t *testing.T, projectName, name, description string) string {
	t.Helper()
	resp := s.do(t, "POST", fmt.Sprintf("/api/v1/projects/%s/features", projectName),
		map[string]string{"name": name, "description": description},
		s.dashboardAuth())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create feature: expected 201, got %d", resp.StatusCode)
	}
	var result map[string]any
	decode(t, resp, &result)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatal("create feature: no id in response")
	}
	return id
}

// startDialog transitions a feature from draft to awaiting_client.
func (s *e2eServer) startDialog(t *testing.T, projectName, featureID string) {
	t.Helper()
	resp := s.do(t, "POST",
		fmt.Sprintf("/api/v1/projects/%s/features/%s/start-dialog", projectName, featureID),
		nil, s.dashboardAuth())
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("start-dialog: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// respond sends a human response.
func (s *e2eServer) respond(t *testing.T, projectName, featureID, response string, final bool) {
	t.Helper()
	resp := s.do(t, "POST",
		fmt.Sprintf("/api/v1/projects/%s/features/%s/respond", projectName, featureID),
		map[string]any{"response": response, "final": final},
		s.dashboardAuth())
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("respond: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// poll calls GET /api/v1/poll with timeout=1 and expects work (200).
func (s *e2eServer) poll(t *testing.T) map[string]any {
	t.Helper()
	resp := s.do(t, "GET", "/api/v1/poll?timeout=1", nil, s.bearerAuth())
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("poll: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	decode(t, resp, &result)
	return result
}

// pollExpectTimeout calls GET /api/v1/poll with timeout=1 and expects 204.
func (s *e2eServer) pollExpectTimeout(t *testing.T) {
	t.Helper()
	resp := s.do(t, "GET", "/api/v1/poll?timeout=1", nil, s.bearerAuth())
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("poll: expected 204 (no work), got %d", resp.StatusCode)
	}
}

// fetchPending calls GET /api/v1/features/{id}/pending and returns the response.
func (s *e2eServer) fetchPending(t *testing.T, featureID string) map[string]any {
	t.Helper()
	resp := s.do(t, "GET", fmt.Sprintf("/api/v1/features/%s/pending", featureID), nil, s.bearerAuth())
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("fetch pending: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	decode(t, resp, &result)
	return result
}

// submitDialog calls POST /api/v1/features/{id}/submit-dialog.
func (s *e2eServer) submitDialog(t *testing.T, featureID, description, questions string) map[string]any {
	t.Helper()
	resp := s.do(t, "POST",
		fmt.Sprintf("/api/v1/features/%s/submit-dialog", featureID),
		map[string]string{"updated_description": description, "questions": questions},
		s.bearerAuth())
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("submit-dialog: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	decode(t, resp, &result)
	return result
}

// featureStatus returns the current status of a feature.
func (s *e2eServer) featureStatus(t *testing.T, projectName, featureID string) string {
	t.Helper()
	resp := s.do(t, "GET",
		fmt.Sprintf("/api/v1/projects/%s/features/%s", projectName, featureID),
		nil, s.dashboardAuth())
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get feature: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	decode(t, resp, &result)
	return result["status"].(string)
}

// TestE2EFullDialogLifecycle exercises the full dialog flow:
// create project/feature → start-dialog → poll → fetch → submit →
// respond → poll → fetch → submit → respond(final) → poll → fetch →
// submit(no questions) → verify fully_specified
func TestE2EFullDialogLifecycle(t *testing.T) {
	s := newE2EServer(t)
	projectName := "testproject"
	s.setupProject(t, projectName)

	featureID := s.createFeature(t, projectName, "Add user profiles",
		"# Add user profiles\nAllow users to create and edit their profiles.")

	// No work yet — feature is still draft.
	s.pollExpectTimeout(t)

	// Start the dialog.
	s.startDialog(t, projectName, featureID)

	// --- Round 1 ---
	// Poll should find work.
	action := s.poll(t)
	if action["action"] != "dialog_step" {
		t.Errorf("expected action=dialog_step, got %v", action["action"])
	}
	if action["feature_id"] != featureID {
		t.Errorf("expected feature_id=%s, got %v", featureID, action["feature_id"])
	}

	// Fetch pending: first round, only feature_description, no questions or user_response.
	pending := s.fetchPending(t, featureID)
	if pending["feature_description"] == "" {
		t.Error("expected non-empty feature_description in first round pending")
	}
	if pending["questions"] != "" && pending["questions"] != nil {
		t.Errorf("expected empty questions in first round pending, got %v", pending["questions"])
	}
	if pending["user_response"] != "" && pending["user_response"] != nil {
		t.Errorf("expected empty user_response in first round pending, got %v", pending["user_response"])
	}
	if pending["iteration"] != float64(0) {
		t.Errorf("expected iteration=0 in first round pending, got %v", pending["iteration"])
	}

	// Client submits round 1 with questions.
	submitResult := s.submitDialog(t, featureID, "# Add user profiles v1\nUpdated description.", "What fields should the profile have?")
	if submitResult["status"] != "awaiting_human" {
		t.Errorf("expected status awaiting_human after submit, got %v", submitResult["status"])
	}

	// No work while awaiting human.
	s.pollExpectTimeout(t)

	// Human responds (not final).
	s.respond(t, projectName, featureID, "Name, bio, avatar.", false)

	// --- Round 2 ---
	action = s.poll(t)
	if action["action"] != "dialog_step" {
		t.Errorf("expected action=dialog_step, got %v", action["action"])
	}

	// Fetch pending: subsequent round, all fields populated.
	pending = s.fetchPending(t, featureID)
	if pending["feature_description"] == "" {
		t.Error("expected non-empty feature_description in round 2")
	}
	if pending["questions"] == "" {
		t.Error("expected non-empty questions in round 2")
	}
	if pending["user_response"] == "" {
		t.Error("expected non-empty user_response in round 2")
	}
	if pending["iteration"] != float64(1) {
		t.Errorf("expected iteration=1 in round 2, got %v", pending["iteration"])
	}

	// Client submits round 2 with more questions.
	submitResult = s.submitDialog(t, featureID, "# Add user profiles v2\nFurther detail.", "Should avatar be required?")
	if submitResult["status"] != "awaiting_human" {
		t.Errorf("expected status awaiting_human after submit, got %v", submitResult["status"])
	}

	// Human responds with final=true.
	s.respond(t, projectName, featureID, "Avatar is optional.", true)

	// --- Final round ---
	action = s.poll(t)
	if action["action"] != "dialog_step" {
		t.Errorf("expected action=dialog_step in final round, got %v", action["action"])
	}

	pending = s.fetchPending(t, featureID)
	if pending["feature_description"] == "" {
		t.Error("expected non-empty feature_description in final round")
	}

	// Client submits final round with no questions → should transition to fully_specified.
	submitResult = s.submitDialog(t, featureID, "# Add user profiles FINAL\nFully specified.", "")
	if submitResult["status"] != "fully_specified" {
		t.Errorf("expected status fully_specified after final submit, got %v", submitResult["status"])
	}

	// Confirm via store.
	if status := s.featureStatus(t, projectName, featureID); status != "fully_specified" {
		t.Errorf("expected feature status fully_specified, got %s", status)
	}

	// No work remaining.
	s.pollExpectTimeout(t)
}

// TestE2EReopenFlow exercises the reopen dialog flow:
// (starts from fully_specified) → reopen → poll → fetch → submit →
// respond(final) → poll → fetch → submit(no questions) → verify fully_specified
func TestE2EReopenFlow(t *testing.T) {
	s := newE2EServer(t)
	projectName := "reopenproject"
	s.setupProject(t, projectName)

	featureID := s.createFeature(t, projectName, "Reopen feature", "Initial description.")

	// Bring to fully_specified: start → submit → respond(final) → submit(no questions).
	s.startDialog(t, projectName, featureID)
	s.poll(t) // consume work
	s.submitDialog(t, featureID, "Updated v1.", "Any questions?")
	s.respond(t, projectName, featureID, "No questions needed.", true)
	s.poll(t) // consume work
	result := s.submitDialog(t, featureID, "Final description.", "")
	if result["status"] != "fully_specified" {
		t.Fatalf("setup: expected fully_specified, got %v", result["status"])
	}

	// --- Reopen ---
	resp := s.do(t, "POST",
		fmt.Sprintf("/api/v1/projects/%s/features/%s/reopen", projectName, featureID),
		map[string]string{"message": "Please add dark mode support."},
		s.dashboardAuth())
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("reopen: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Poll: work should be available (dialog_step).
	action := s.poll(t)
	if action["action"] != "dialog_step" {
		t.Errorf("expected action=dialog_step after reopen, got %v", action["action"])
	}

	// Fetch pending after reopen: questions empty, user_response = reopen message.
	pending := s.fetchPending(t, featureID)
	if pending["feature_description"] == "" {
		t.Error("expected non-empty feature_description after reopen")
	}
	if pending["questions"] != "" && pending["questions"] != nil {
		t.Errorf("expected empty questions after reopen, got %v", pending["questions"])
	}
	if pending["user_response"] == "" {
		t.Error("expected non-empty user_response (reopen message) after reopen")
	}

	// Client submits after reopen.
	submitResult := s.submitDialog(t, featureID, "Updated after reopen.", "Does dark mode affect mobile?")
	if submitResult["status"] != "awaiting_human" {
		t.Errorf("expected awaiting_human after reopen submit, got %v", submitResult["status"])
	}

	// Human responds with final=true.
	s.respond(t, projectName, featureID, "Dark mode applies to all platforms.", true)

	// Poll for final round.
	s.poll(t)

	// Submit final round (no questions) → fully_specified.
	finalResult := s.submitDialog(t, featureID, "Final after reopen.", "")
	if finalResult["status"] != "fully_specified" {
		t.Errorf("expected fully_specified after reopen final submit, got %v", finalResult["status"])
	}

	if status := s.featureStatus(t, projectName, featureID); status != "fully_specified" {
		t.Errorf("expected feature status fully_specified after reopen flow, got %s", status)
	}
}

// TestE2EConnectivity verifies that poll requests update the connectivity status.
func TestE2EConnectivity(t *testing.T) {
	s := newE2EServer(t)
	projectName := "connproject"
	s.setupProject(t, projectName)

	// Before any poll: no connectivity info.
	resp := s.do(t, "GET", "/api/v1/project", nil, s.bearerAuth())
	var proj map[string]any
	decode(t, resp, &proj)
	if proj["connectivity"] != nil && proj["connectivity"] != "" {
		t.Errorf("expected no connectivity before first poll, got %v", proj["connectivity"])
	}

	// Poll (will timeout since no work, but records timestamp).
	s.do(t, "GET", "/api/v1/poll?timeout=1", nil, s.bearerAuth()).Body.Close()

	// After poll: should be "Connected".
	resp = s.do(t, "GET", "/api/v1/project", nil, s.bearerAuth())
	decode(t, resp, &proj)
	if connectivity, ok := proj["connectivity"].(string); !ok || !strings.HasPrefix(connectivity, "Connected") {
		t.Errorf("expected connectivity to start with Connected after poll, got %v", proj["connectivity"])
	}

	// Dashboard should also see connectivity.
	resp = s.do(t, "GET", fmt.Sprintf("/api/v1/projects/%s", projectName), nil, s.dashboardAuth())
	decode(t, resp, &proj)
	if connectivity, ok := proj["connectivity"].(string); !ok || !strings.HasPrefix(connectivity, "Connected") {
		t.Errorf("expected connectivity to start with Connected in dashboard view, got %v", proj["connectivity"])
	}
}
