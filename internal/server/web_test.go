package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vector76/backlog_manager/internal/model"
)

// webRequest sends a request with optional session cookie.
func webRequest(t *testing.T, srv *http.Server, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	return w
}

// login performs a login and returns the session cookie.
func loginWeb(t *testing.T, srv *http.Server) *http.Cookie {
	t.Helper()
	body := url.Values{"username": {"admin"}, "password": {"secret"}}.Encode()
	w := webRequest(t, srv, "POST", "/login", body, nil)
	if w.Code != http.StatusFound {
		t.Fatalf("login: expected 302, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "bm_session" {
			return c
		}
	}
	t.Fatal("login: no session cookie in response")
	return nil
}

// loginWebAsViewer performs a viewer login and returns the session cookie.
func loginWebAsViewer(t *testing.T, srv *http.Server) *http.Cookie {
	t.Helper()
	body := url.Values{"username": {"viewer"}, "password": {"viewpass"}}.Encode()
	w := webRequest(t, srv, "POST", "/login", body, nil)
	if w.Code != http.StatusFound {
		t.Fatalf("loginWebAsViewer: expected 302, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "bm_session" {
			return c
		}
	}
	t.Fatal("loginWebAsViewer: no session cookie in response")
	return nil
}

// TestWebLoginPage checks that the login page is accessible.
func TestWebLoginPage(t *testing.T) {
	srv, _ := newTestServer(t)
	w := webRequest(t, srv, "GET", "/login", "", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Sign in") {
		t.Errorf("expected login form in response body")
	}
}

// TestWebLoginInvalidCredentials checks that bad credentials are rejected.
func TestWebLoginInvalidCredentials(t *testing.T) {
	srv, _ := newTestServer(t)
	body := url.Values{"username": {"admin"}, "password": {"wrong"}}.Encode()
	w := webRequest(t, srv, "POST", "/login", body, nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid username or password") {
		t.Errorf("expected error message in response body, got: %s", w.Body.String()[:200])
	}
}

// TestWebLoginSuccess checks that correct credentials set a session cookie and redirect.
func TestWebLoginSuccess(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)
	if cookie == nil {
		t.Fatal("expected session cookie")
	}
	if cookie.Value == "" {
		t.Error("expected non-empty session ID")
	}
}

// TestWebDashboardRequiresAuth checks that unauthenticated requests are redirected to /login.
func TestWebDashboardRequiresAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := webRequest(t, srv, "GET", "/", "", nil)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect to login, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}
}

// TestWebDashboardAuthenticated checks that an authenticated user can view the dashboard.
func TestWebDashboardAuthenticated(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)
	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Projects") {
		t.Errorf("expected dashboard content in body")
	}
}

// TestWebDashboardWithProjects checks dashboard rendering when projects exist (regression for JustCreated bug).
func TestWebDashboardWithProjects(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("existing-project", "tok-existing")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// The project name must appear in the fully rendered page (after the range loop completes).
	if !strings.Contains(body, "existing-project") {
		t.Errorf("expected project name in dashboard body")
	}
	// Verify the page is complete (has closing </html> tag).
	if !strings.Contains(body, "</html>") {
		t.Errorf("expected complete HTML in dashboard response")
	}
}

// TestWebDashboardNoViewButton checks that the dashboard does not render a View button for features.
func TestWebDashboardNoViewButton(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("view-btn-project", "tok-view")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("view-btn-project", "My Feature Name", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, ">View<") {
		t.Errorf("expected no View button in dashboard, but found one")
	}
	if !strings.Contains(body, "/project/view-btn-project/feature/"+f.ID) {
		t.Errorf("expected feature link in dashboard body")
	}
	if !strings.Contains(body, "My Feature Name") {
		t.Errorf("expected feature name in dashboard body")
	}
}

// TestWebDashboardProjectDataAttribute checks that projects have a data-project attribute for localStorage persistence.
func TestWebDashboardProjectDataAttribute(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("attr-test-project", "tok-attr")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `data-project="`) {
		t.Errorf("expected data-project attribute in dashboard body")
	}
}

// TestWebDashboardLivePageAttribute checks that the dashboard renders the data-live-page="dashboard" marker.
func TestWebDashboardLivePageAttribute(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `data-live-page="dashboard"`) {
		t.Errorf("expected data-live-page=\"dashboard\" attribute in dashboard body")
	}
}

// TestWebLogout checks that logout clears the session and redirects to /login.
func TestWebLogout(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	w := webRequest(t, srv, "GET", "/logout", "", cookie)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 after logout, got %d", w.Code)
	}

	// After logout, the session should be invalid.
	w2 := webRequest(t, srv, "GET", "/", "", cookie)
	if w2.Code != http.StatusFound {
		t.Errorf("expected redirect after using invalidated session, got %d", w2.Code)
	}
}

// TestWebCreateProject checks that a project can be created via the web form.
func TestWebCreateProject(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	body := url.Values{"name": {"web-test-project"}}.Encode()
	w := webRequest(t, srv, "POST", "/projects", body, cookie)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/?created=web-test-project&token=") {
		t.Errorf("expected redirect with token, got: %s", loc)
	}
}

// TestWebDashboardShowsNewProjectToken checks the dashboard banner after project creation.
func TestWebDashboardShowsNewProjectToken(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	// Create project via web form.
	body := url.Values{"name": {"token-test"}}.Encode()
	w := webRequest(t, srv, "POST", "/projects", body, cookie)
	loc := w.Header().Get("Location")

	// Follow redirect to dashboard.
	w2 := webRequest(t, srv, "GET", loc, "", cookie)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "token-test") {
		t.Errorf("expected project name in dashboard, got: %s", w2.Body.String()[:300])
	}
	if !strings.Contains(w2.Body.String(), "created") {
		t.Errorf("expected token creation notice, got: %s", w2.Body.String()[:300])
	}
}

// TestWebNewFeaturePage checks that the new feature form loads.
func TestWebNewFeaturePage(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("feat-project", "tok")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/feat-project/new", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Feature name") {
		t.Errorf("expected feature form in body")
	}
}

// TestWebNewFeatureCancelLink checks that the Cancel button on the new feature form links to the dashboard.
func TestWebNewFeatureCancelLink(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("cancel-link-project", "tok")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/cancel-link-project/new", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `href="/"`) || !strings.Contains(w.Body.String(), "Cancel") {
		t.Errorf("expected cancel link pointing to dashboard root, got: %s", w.Body.String())
	}
}

// TestWebCreateFeature checks that a feature can be created via the web form.
func TestWebCreateFeature(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("create-feat-project", "tok2")
	if err != nil {
		t.Fatal(err)
	}

	body := url.Values{
		"name":        {"My New Feature"},
		"description": {"# Feature\n\nThis is a description."},
	}.Encode()
	w := webRequest(t, srv, "POST", "/project/create-feat-project/features", body, cookie)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	prefix := "/project/create-feat-project/feature/"
	if !strings.HasPrefix(loc, prefix) || len(loc) <= len(prefix) {
		t.Errorf("expected redirect to feature detail page, got: %s", loc)
	}

	// Verify feature detail page is correctly served.
	w2 := webRequest(t, srv, "GET", loc, "", cookie)
	if !strings.Contains(w2.Body.String(), "My New Feature") {
		t.Errorf("expected feature name in detail page")
	}
}

// TestWebFeatureDetail checks that the feature detail page renders with markdown.
func TestWebFeatureDetail(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("md-project", "tok3")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("md-project", "MD Feature", "## Overview\n\nHello **world**.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/md-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "MD Feature") {
		t.Errorf("expected feature name in page")
	}
	// Markdown should be rendered as HTML.
	if !strings.Contains(body, "<strong>world</strong>") {
		t.Errorf("expected rendered markdown HTML in page")
	}
}

// TestWebFeatureDetailDraftActions checks that draft action controls appear on the feature page.
func TestWebFeatureDetailDraftActions(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("act-project", "tok4")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("act-project", "Draft Feature", "Initial description here.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/act-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Start Dialog") {
		t.Errorf("expected Start Dialog button for draft feature")
	}
	if !strings.Contains(body, "Edit Description") {
		t.Errorf("expected Edit Description control for draft feature")
	}
	if !strings.Contains(body, "Initial description here.") {
		t.Errorf("expected current description in page")
	}
}

// TestWebFeatureUpdateDescription checks that the description update form works for draft features.
func TestWebFeatureUpdateDescription(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("upd-project", "tok5")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("upd-project", "Upd Feature", "Old description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	body := url.Values{"description": {"Updated description content."}}.Encode()
	w := webRequest(t, srv, "POST", "/project/upd-project/feature/"+f.ID+"/description", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}

	// Follow redirect and verify updated description.
	w2 := webRequest(t, srv, "GET", "/project/upd-project/feature/"+f.ID, "", cookie)
	if !strings.Contains(w2.Body.String(), "Updated description content.") {
		t.Errorf("expected updated description in feature page")
	}
}

// TestWebFeatureStartDialog checks that posting to start-dialog transitions the feature.
func TestWebFeatureStartDialog(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("dialog-project", "tok6")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("dialog-project", "Dialog Feature", "Some description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "POST", "/project/dialog-project/feature/"+f.ID+"/start-dialog", "", cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}

	// Check the feature status changed.
	updated, err := st.GetFeature("dialog-project", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status.String() != "awaiting_client" {
		t.Errorf("expected awaiting_client after start-dialog, got %s", updated.Status)
	}
}

// TestWebFeatureAwaitingHumanActions checks that awaiting_human features show respond controls.
func TestWebFeatureAwaitingHumanActions(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("ah-project", "tok7")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("ah-project", "AH Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Transition to awaiting_human via start-dialog then simulate client submission.
	if err := st.StartDialog("ah-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("ah-project", f.ID, "Revised description.", "What is the scope?"); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/ah-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Send Response") {
		t.Errorf("expected Send Response button for awaiting_human feature")
	}
	if !strings.Contains(body, "Final answer") {
		t.Errorf("expected Final answer checkbox for awaiting_human feature")
	}
	if !strings.Contains(body, "What is the scope?") {
		t.Errorf("expected client questions displayed prominently")
	}
}

// TestWebFeatureRespond checks that posting a response transitions the feature.
func TestWebFeatureRespond(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("resp-project", "tok8")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("resp-project", "Resp Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.StartDialog("resp-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("resp-project", f.ID, "Revised.", "Questions?"); err != nil {
		t.Fatal(err)
	}

	body := url.Values{"response": {"My answer."}, "final": {"false"}}.Encode()
	w := webRequest(t, srv, "POST", "/project/resp-project/feature/"+f.ID+"/respond", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}

	updated, err := st.GetFeature("resp-project", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status.String() != "awaiting_client" {
		t.Errorf("expected awaiting_client after respond, got %s", updated.Status)
	}
}

// TestWebFeatureRespondFinal checks that posting a final response sets is_final.
func TestWebFeatureRespondFinal(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("final-project", "tok9")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("final-project", "Final Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.StartDialog("final-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("final-project", f.ID, "Revised.", "Last questions?"); err != nil {
		t.Fatal(err)
	}

	body := url.Values{"response": {"Final answer."}, "final": {"on"}}.Encode()
	w := webRequest(t, srv, "POST", "/project/final-project/feature/"+f.ID+"/respond", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}

	updated, err := st.GetFeature("final-project", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Should be marked as final in the iteration metadata.
	foundFinal := false
	for _, it := range updated.Iterations {
		if it.IsFinal {
			foundFinal = true
		}
	}
	if !foundFinal {
		t.Errorf("expected IsFinal=true in feature iteration metadata")
	}
}

// TestWebFeatureFullySpecifiedActions checks that fully_specified features show reopen and disabled generate controls.
func TestWebFeatureFullySpecifiedActions(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("fs-project", "tokA")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("fs-project", "FS Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Transition to fully_specified.
	if err := st.StartDialog("fs-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("fs-project", f.ID, "Revised.", ""); err != nil {
		t.Fatal(err)
	}
	// Submit final response so next client round becomes fully_specified.
	if err := st.RespondToDialog("fs-project", f.ID, "Final.", true); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("fs-project", f.ID, "Final desc.", ""); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/fs-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Reopen") {
		t.Errorf("expected Reopen control for fully_specified feature")
	}
	if !strings.Contains(body, "Generate Now") {
		t.Errorf("expected disabled Generate Now button for fully_specified feature")
	}
}

// TestWebFeatureReopen checks that posting to reopen transitions the feature back to awaiting_client.
func TestWebFeatureReopen(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("reopen-project", "tokB")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("reopen-project", "Reopen Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.StartDialog("reopen-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("reopen-project", f.ID, "Revised.", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.RespondToDialog("reopen-project", f.ID, "Final.", true); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("reopen-project", f.ID, "Final desc.", ""); err != nil {
		t.Fatal(err)
	}

	body := url.Values{"message": {"Need to clarify one thing."}}.Encode()
	w := webRequest(t, srv, "POST", "/project/reopen-project/feature/"+f.ID+"/reopen", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}

	updated, err := st.GetFeature("reopen-project", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status.String() != "awaiting_client" {
		t.Errorf("expected awaiting_client after reopen, got %s", updated.Status)
	}
}

// TestWebFeatureCurrentDescriptionAfterReopen checks that the current description shows the
// last client-provided description, not the initial one, after a reopen (which creates a new
// iteration with no description yet).
func TestWebFeatureCurrentDescriptionAfterReopen(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("reopen-desc-project", "tokD")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("reopen-desc-project", "Desc Feature", "Original description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Full dialog cycle to fully_specified.
	if err := st.StartDialog("reopen-desc-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("reopen-desc-project", f.ID, "Client revised description.", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.RespondToDialog("reopen-desc-project", f.ID, "Final.", true); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("reopen-desc-project", f.ID, "Final client desc.", ""); err != nil {
		t.Fatal(err)
	}

	// Reopen — creates a new round with only a response file, no description yet.
	if err := st.ReopenDialog("reopen-desc-project", f.ID, "Please revisit."); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/reopen-desc-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Current description should be the latest client-provided one, not the original.
	if !strings.Contains(body, "Final client desc.") {
		t.Errorf("expected latest client description as current description after reopen")
	}
	if strings.Contains(body, "Original description.") && !strings.Contains(body, "Final client desc.") {
		t.Errorf("should not fall back to original description after rounds of dialog")
	}
}

// TestWebFeatureDialogHistory checks that dialog iterations render with collapsible rounds.
func TestWebFeatureDialogHistory(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("hist-project", "tokC")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("hist-project", "History Feature", "Initial.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.StartDialog("hist-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("hist-project", f.ID, "Updated desc.", "What is the budget?"); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/hist-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Round 1") {
		t.Errorf("expected Round 1 in dialog history")
	}
	if !strings.Contains(body, "What is the budget?") {
		t.Errorf("expected client questions in dialog history")
	}
	if !strings.Contains(body, "Updated desc.") {
		t.Errorf("expected revised description in dialog history")
	}
}

// TestWebFeatureWaitingStatus checks that the waiting/generating status card renders
// (exercises the multi-status {{if or ...}} branch in the template).
func TestWebFeatureWaitingStatus(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("wait-project", "tokE")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("wait-project", "Wait Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Reach fully_specified, then transition to waiting via TransitionStatus.
	if err := st.StartDialog("wait-project", f.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("wait-project", f.ID, "Revised.", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.RespondToDialog("wait-project", f.ID, "Final.", true); err != nil {
		t.Fatal(err)
	}
	if err := st.SubmitClientDialog("wait-project", f.ID, "Final desc.", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("wait-project", f.ID, model.StatusWaiting); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/wait-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Halt") {
		t.Errorf("expected Halt placeholder for waiting feature")
	}
	// Should not show any action forms.
	if strings.Contains(body, "Start Dialog") || strings.Contains(body, "Send Response") || strings.Contains(body, "Reopen") {
		t.Errorf("waiting feature should not show dialog action controls")
	}
}

// TestWebFeatureDetailLivePageAttributes checks that the feature page exposes data attributes for JS live update.
func TestWebFeatureDetailLivePageAttributes(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("live-attr-project", "tok-live")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("live-attr-project", "Live Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/live-attr-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `data-live-page="feature"`) {
		t.Errorf("expected data-live-page=\"feature\" attribute in feature page")
	}
	if !strings.Contains(body, `data-project="live-attr-project"`) {
		t.Errorf("expected data-project attribute in feature page")
	}
	if !strings.Contains(body, `data-feature-id="`+f.ID+`"`) {
		t.Errorf("expected data-feature-id attribute in feature page")
	}
}

// TestWebFeatureDetailActionSectionContainer checks that the action-section div is present.
func TestWebFeatureDetailActionSectionContainer(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("action-sec-project", "tok-action")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("action-sec-project", "Action Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/action-sec-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="action-section"`) {
		t.Errorf("expected id=\"action-section\" div in feature page")
	}
}

// TestWebFeatureDetailDialogRoundsContainer checks that the dialog-rounds div is always present.
func TestWebFeatureDetailDialogRoundsContainer(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("rounds-project", "tok-rounds")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("rounds-project", "Rounds Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// No iterations yet — container should still be present.
	w := webRequest(t, srv, "GET", "/project/rounds-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="dialog-rounds"`) {
		t.Errorf("expected id=\"dialog-rounds\" div in feature page (even with no iterations)")
	}
}

// TestWebStaticFiles checks that static assets are served.
func TestWebStaticFiles(t *testing.T) {
	srv, _ := newTestServer(t)
	w := webRequest(t, srv, "GET", "/static/style.css", "", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for static file, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("expected text/css content type, got %s", ct)
	}
}

// TestWebStaticPathTraversal checks that path traversal cannot reach template files.
func TestWebStaticPathTraversal(t *testing.T) {
	srv, _ := newTestServer(t)

	// These should not expose the template sources.
	traversalPaths := []string{
		"/static/../templates/base.html",
		"/static/%2e%2e/templates/base.html",
	}
	for _, p := range traversalPaths {
		w := webRequest(t, srv, "GET", p, "", nil)
		if w.Code == http.StatusOK {
			t.Errorf("path traversal %q returned 200, expected non-200", p)
		}
	}
}

// TestWebLoginAlreadyLoggedIn checks that an already-logged-in user is redirected to dashboard.
func TestWebLoginAlreadyLoggedIn(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	w := webRequest(t, srv, "GET", "/login", "", cookie)
	if w.Code != http.StatusFound {
		t.Errorf("expected redirect for already-logged-in user, got %d", w.Code)
	}
}

// TestWebDashboardActiveBeforeTerminal checks that active features appear before terminal ones.
func TestWebDashboardActiveBeforeTerminal(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("order-test", "tok-order")
	if err != nil {
		t.Fatal(err)
	}
	termFeat, err := st.CreateFeature("order-test", "terminal-feat", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("order-test", termFeat.ID, model.StatusAbandoned); err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFeature("order-test", "active-feat", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Index(body, "active-feat") >= strings.Index(body, "terminal-feat") {
		t.Errorf("expected active-feat to appear before terminal-feat in dashboard")
	}
}

// TestWebDashboardCreatedAtSortActiveFeatures checks that newer active features appear before older ones.
func TestWebDashboardCreatedAtSortActiveFeatures(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("sort-active", "tok-sort-active")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFeature("sort-active", "older-active", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	_, err = st.CreateFeature("sort-active", "newer-active", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Index(body, "newer-active") >= strings.Index(body, "older-active") {
		t.Errorf("expected newer-active to appear before older-active in dashboard")
	}
}

// TestWebDashboardCreatedAtSortTerminalFeatures checks that newer terminal features appear before older ones.
func TestWebDashboardCreatedAtSortTerminalFeatures(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("sort-terminal", "tok-sort-terminal")
	if err != nil {
		t.Fatal(err)
	}
	olderFeat, err := st.CreateFeature("sort-terminal", "older-abandoned", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("sort-terminal", olderFeat.ID, model.StatusAbandoned); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	newerFeat, err := st.CreateFeature("sort-terminal", "newer-abandoned", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("sort-terminal", newerFeat.ID, model.StatusAbandoned); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Index(body, "newer-abandoned") >= strings.Index(body, "older-abandoned") {
		t.Errorf("expected newer-abandoned to appear before older-abandoned in dashboard")
	}
}

// TestWebDashboardTerminalOnlyNoEmptyState checks that terminal-only projects don't show empty state.
func TestWebDashboardTerminalOnlyNoEmptyState(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("terminal-only", "tok-terminal-only")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("terminal-only", "sole-feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("terminal-only", f.ID, model.StatusAbandoned); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "sole-feature") {
		t.Errorf("expected sole-feature in dashboard body")
	}
	if strings.Contains(body, "No features yet") {
		t.Errorf("expected no empty-state message when terminal features exist")
	}
}

// TestWebDashboardNoStatusGroupTitle checks that the dashboard does not render status-group headers.
func TestWebDashboardNoStatusGroupTitle(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("no-groups", "tok-no-groups")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFeature("no-groups", "A Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "status-group-title") {
		t.Errorf("expected no status-group-title class in dashboard body")
	}
	if strings.Contains(body, "status-group") {
		t.Errorf("expected no status-group class in dashboard body")
	}
}

// TestWebDashboardBeadInfoPreserved checks that a BeadsCreated feature appears in the dashboard table.
func TestWebDashboardBeadInfoPreserved(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("bead-info-test", "tok-bead-info")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("bead-info-test", "Bead Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("bead-info-test", f.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("bead-info-test", f.ID, model.StatusFullySpecified); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("bead-info-test", f.ID, model.StatusReadyToGenerate); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("bead-info-test", f.ID, model.StatusGenerating); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus("bead-info-test", f.ID, model.StatusBeadsCreated); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Bead Feature") {
		t.Errorf("expected Bead Feature in dashboard body")
	}
}

// TestWebNewFeatureFormHasDirectToBeadCheckbox checks that the new feature form includes the checkbox.
func TestWebNewFeatureFormHasDirectToBeadCheckbox(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("dtb-form-project", "tok-dtb-form")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/dtb-form-project/new", "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `name="direct_to_bead"`) {
		t.Errorf("expected direct_to_bead checkbox in new feature form")
	}
	if !strings.Contains(body, "Direct to bead") {
		t.Errorf("expected 'Direct to bead' label text in new feature form")
	}
}

// TestWebFeatureDetailShowsDirectToBead checks that the detail page shows "Direct to bead" when set.
func TestWebFeatureDetailShowsDirectToBead(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("dtb-detail-project", "tok-dtb-detail")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("dtb-detail-project", "DTB Feature", "Description.", true, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/dtb-detail-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Direct to bead") {
		t.Errorf("expected 'Direct to bead' text in feature detail page when DirectToBead=true")
	}
}

// TestWebFeatureDetailNoDirectToBeadWhenFalse checks that the detail page omits "Direct to bead" when not set.
func TestWebFeatureDetailNoDirectToBeadWhenFalse(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("no-dtb-project", "tok-no-dtb")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("no-dtb-project", "Normal Feature", "Description.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/no-dtb-project/feature/"+f.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "Direct to bead") {
		t.Errorf("expected no 'Direct to bead' text in feature detail page when DirectToBead=false")
	}
}

// TestWebCreateFeatureWithDirectToBead checks that the web form correctly passes direct_to_bead.
func TestWebCreateFeatureWithDirectToBead(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("dtb-create-project", "tok-dtb-create")
	if err != nil {
		t.Fatal(err)
	}

	body := url.Values{
		"name":           {"DTB Feature"},
		"description":    {"Some description."},
		"direct_to_bead": {"true"},
	}.Encode()
	w := webRequest(t, srv, "POST", "/project/dtb-create-project/features", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")

	// Follow redirect and verify "Direct to bead" appears on the detail page.
	w2 := webRequest(t, srv, "GET", loc, "", cookie)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for feature detail, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "Direct to bead") {
		t.Errorf("expected 'Direct to bead' text after creating feature with direct_to_bead=true")
	}
}

// TestWebFeatureDetailGenerateAfterDropdown checks that the Generate After dropdown
// includes only features in the explicitly allowed statuses and excludes all others.
func TestWebFeatureDetailGenerateAfterDropdown(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	const proj = "ga-dropdown-project"
	if _, err := st.CreateProject(proj, "tok-ga"); err != nil {
		t.Fatal(err)
	}

	// Create and advance subject feature to fully_specified.
	subject, err := st.CreateFeature(proj, "Subject Feature", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, subject.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, subject.ID, model.StatusFullySpecified); err != nil {
		t.Fatal(err)
	}

	// Helper to chain transitions.
	transitionThrough := func(featureID string, statuses ...model.FeatureStatus) {
		t.Helper()
		for _, s := range statuses {
			if err := st.TransitionStatus(proj, featureID, s); err != nil {
				t.Fatalf("TransitionStatus(%v) for %s: %v", s, featureID, err)
			}
		}
	}

	// Create one sibling per status.
	createSibling := func(name string) string {
		t.Helper()
		f, err := st.CreateFeature(proj, name, "desc", false, "")
		if err != nil {
			t.Fatalf("CreateFeature %q: %v", name, err)
		}
		return f.ID
	}

	createSibling("sibling-draft") // stays in draft

	awaitingClientID := createSibling("sibling-awaiting-client")
	transitionThrough(awaitingClientID, model.StatusAwaitingClient)

	awaitingHumanID := createSibling("sibling-awaiting-human")
	transitionThrough(awaitingHumanID, model.StatusAwaitingClient, model.StatusAwaitingHuman)

	fullySpecifiedID := createSibling("sibling-fully-specified")
	transitionThrough(fullySpecifiedID, model.StatusAwaitingClient, model.StatusFullySpecified)

	waitingID := createSibling("sibling-waiting")
	transitionThrough(waitingID, model.StatusAwaitingClient, model.StatusFullySpecified, model.StatusWaiting)

	readyToGenerateID := createSibling("sibling-ready-to-generate")
	transitionThrough(readyToGenerateID, model.StatusAwaitingClient, model.StatusFullySpecified, model.StatusReadyToGenerate)

	generatingID := createSibling("sibling-generating")
	transitionThrough(generatingID, model.StatusAwaitingClient, model.StatusFullySpecified, model.StatusReadyToGenerate, model.StatusGenerating)

	beadsCreatedID := createSibling("sibling-beads-created")
	transitionThrough(beadsCreatedID, model.StatusAwaitingClient, model.StatusFullySpecified, model.StatusReadyToGenerate, model.StatusGenerating, model.StatusBeadsCreated)

	doneID := createSibling("sibling-done")
	transitionThrough(doneID, model.StatusAwaitingClient, model.StatusFullySpecified, model.StatusReadyToGenerate, model.StatusGenerating, model.StatusBeadsCreated, model.StatusDone)

	abandonedID := createSibling("sibling-abandoned")
	transitionThrough(abandonedID, model.StatusAbandoned)

	haltedID := createSibling("sibling-halted")
	transitionThrough(haltedID, model.StatusHalted)

	// GET the subject feature's detail page.
	w := webRequest(t, srv, "GET", "/project/"+proj+"/feature/"+subject.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	// Features that SHOULD appear in the generate-after dropdown.
	for _, name := range []string{
		"sibling-fully-specified",
		"sibling-waiting",
		"sibling-ready-to-generate",
		"sibling-generating",
		"sibling-beads-created",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("expected %q to appear in generate-after dropdown, but it was absent", name)
		}
	}

	// Features that MUST NOT appear in the generate-after dropdown.
	for _, name := range []string{
		"sibling-draft",
		"sibling-awaiting-client",
		"sibling-awaiting-human",
		"sibling-done",
		"sibling-abandoned",
		"sibling-halted",
	} {
		if strings.Contains(body, name) {
			t.Errorf("expected %q to be absent from generate-after dropdown, but it was present", name)
		}
	}
}

// TestWebFeatureDetailGenerateAfterDropdown_NoEligibleFeatures checks that the
// generate-after form is absent when there are no eligible sibling features.
func TestWebFeatureDetailGenerateAfterDropdown_NoEligibleFeatures(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	const proj = "ga-no-siblings-project"
	if _, err := st.CreateProject(proj, "tok-ga-ns"); err != nil {
		t.Fatal(err)
	}

	// Create subject feature and advance to fully_specified (no siblings).
	subject, err := st.CreateFeature(proj, "Lonely Feature", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, subject.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj, subject.ID, model.StatusFullySpecified); err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/"+proj+"/feature/"+subject.ID, "", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	// The generate-after form must not appear when there are no eligible siblings.
	if strings.Contains(body, "generate-after") {
		t.Errorf("expected generate-after form to be absent when no eligible siblings exist")
	}
}

// TestWebDeleteDraftFeature checks that a draft feature can be deleted via the web route.
func TestWebDeleteDraftFeature(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	if _, err := st.CreateProject("del-draft-project", "tok-del-draft"); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("del-draft-project", "Draft To Delete", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "POST", "/project/del-draft-project/feature/"+f.ID+"/delete", "", cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %s", loc)
	}

	// Feature should no longer exist.
	got := doRequest(t, srv, "GET", "/api/v1/projects/del-draft-project/features/"+f.ID, nil, basicAuth("admin", "secret"))
	if got.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", got.Code)
	}
}

// TestWebDeleteNonDraftFeature_Rejected checks that deleting a non-draft feature is rejected.
func TestWebDeleteNonDraftFeature_Rejected(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	if _, err := st.CreateProject("del-nondraft-project", "tok-del-nondraft"); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("del-nondraft-project", "Non-Draft Feature", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Transition out of draft.
	if err := st.StartDialog("del-nondraft-project", f.ID); err != nil {
		t.Fatal(err)
	}

	featurePage := "/project/del-nondraft-project/feature/" + f.ID
	w := webRequest(t, srv, "POST", featurePage+"/delete", "", cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != featurePage {
		t.Errorf("expected redirect back to feature page %s, got %s", featurePage, loc)
	}

	// Feature should still exist.
	got := doRequest(t, srv, "GET", "/api/v1/projects/del-nondraft-project/features/"+f.ID, nil, basicAuth("admin", "secret"))
	if got.Code != http.StatusOK {
		t.Errorf("expected feature to still exist (200), got %d", got.Code)
	}
}

// TestWebRenameFeature_Valid checks that a valid rename POST redirects back and updates the name.
func TestWebRenameFeature_Valid(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	if _, err := st.CreateProject("rename-project", "tok-rename"); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("rename-project", "Original Name", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	featurePage := "/project/rename-project/feature/" + f.ID
	body := url.Values{"name": {"New Name"}}.Encode()
	w := webRequest(t, srv, "POST", featurePage+"/rename", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != featurePage {
		t.Errorf("expected redirect to feature page %s, got %s", featurePage, loc)
	}

	// Verify name was updated.
	got := doRequest(t, srv, "GET", "/api/v1/projects/rename-project/features/"+f.ID, nil, basicAuth("admin", "secret"))
	if got.Code != http.StatusOK {
		t.Fatalf("expected 200 getting feature, got %d", got.Code)
	}
	if !strings.Contains(got.Body.String(), "New Name") {
		t.Errorf("expected updated name in response, got: %s", got.Body.String())
	}
}

// TestWebRenameFeature_Empty checks that an empty name redirects back without changing the name.
func TestWebRenameFeature_Empty(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	if _, err := st.CreateProject("rename-empty-project", "tok-rename-empty"); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("rename-empty-project", "Unchanged Name", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	featurePage := "/project/rename-empty-project/feature/" + f.ID
	body := url.Values{"name": {""}}.Encode()
	w := webRequest(t, srv, "POST", featurePage+"/rename", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != featurePage {
		t.Errorf("expected redirect to feature page %s, got %s", featurePage, loc)
	}

	// Verify name was NOT changed.
	got := doRequest(t, srv, "GET", "/api/v1/projects/rename-empty-project/features/"+f.ID, nil, basicAuth("admin", "secret"))
	if got.Code != http.StatusOK {
		t.Fatalf("expected 200 getting feature, got %d", got.Code)
	}
	if !strings.Contains(got.Body.String(), "Unchanged Name") {
		t.Errorf("expected original name in response, got: %s", got.Body.String())
	}
}

// TestWebRenameFeature_NonDraftStatus checks that rename works for non-draft features.
func TestWebRenameFeature_NonDraftStatus(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	if _, err := st.CreateProject("rename-nondraft-project", "tok-rename-nondraft"); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("rename-nondraft-project", "Draft Name", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Transition out of draft.
	if err := st.StartDialog("rename-nondraft-project", f.ID); err != nil {
		t.Fatal(err)
	}

	featurePage := "/project/rename-nondraft-project/feature/" + f.ID
	body := url.Values{"name": {"Non-Draft New Name"}}.Encode()
	w := webRequest(t, srv, "POST", featurePage+"/rename", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != featurePage {
		t.Errorf("expected redirect to feature page %s, got %s", featurePage, loc)
	}

	// Verify name was updated.
	got := doRequest(t, srv, "GET", "/api/v1/projects/rename-nondraft-project/features/"+f.ID, nil, basicAuth("admin", "secret"))
	if got.Code != http.StatusOK {
		t.Fatalf("expected 200 getting feature, got %d", got.Code)
	}
	if !strings.Contains(got.Body.String(), "Non-Draft New Name") {
		t.Errorf("expected updated name in response, got: %s", got.Body.String())
	}
}

// TestWebRenameFeature_WhitespaceOnly checks that a whitespace-only name is rejected.
func TestWebRenameFeature_WhitespaceOnly(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	if _, err := st.CreateProject("rename-ws-project", "tok-rename-ws"); err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("rename-ws-project", "Original Name", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	featurePage := "/project/rename-ws-project/feature/" + f.ID
	body := url.Values{"name": {"   "}}.Encode()
	w := webRequest(t, srv, "POST", featurePage+"/rename", body, cookie)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != featurePage {
		t.Errorf("expected redirect to feature page %s, got %s", featurePage, loc)
	}

	// Verify name was NOT changed.
	got := doRequest(t, srv, "GET", "/api/v1/projects/rename-ws-project/features/"+f.ID, nil, basicAuth("admin", "secret"))
	if got.Code != http.StatusOK {
		t.Fatalf("expected 200 getting feature, got %d", got.Code)
	}
	if !strings.Contains(got.Body.String(), "Original Name") {
		t.Errorf("expected original name in response, got: %s", got.Body.String())
	}
}

// TestWebAdminLoginPreserved checks that admin credentials still work and produce a valid session.
func TestWebAdminLoginPreserved(t *testing.T) {
	srv, _ := newTestServerWithViewer(t)
	cookie := loginWeb(t, srv)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("expected non-empty session cookie for admin")
	}
	// Verify session grants dashboard access.
	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin session, got %d", w.Code)
	}
}

// TestWebViewerLoginSuccess checks that viewer credentials succeed and produce a valid session.
func TestWebViewerLoginSuccess(t *testing.T) {
	srv, _ := newTestServerWithViewer(t)
	cookie := loginWebAsViewer(t, srv)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("expected non-empty session cookie for viewer")
	}
	// Verify session grants dashboard access.
	w := webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for viewer session, got %d", w.Code)
	}
}

// TestWebViewerCredentialsRejectedWhenNotConfigured checks that viewer credentials are rejected
// when viewer config fields are empty.
func TestWebViewerCredentialsRejectedWhenNotConfigured(t *testing.T) {
	srv, _ := newTestServer(t) // no viewer credentials configured
	body := url.Values{"username": {"viewer"}, "password": {"viewpass"}}.Encode()
	w := webRequest(t, srv, "POST", "/login", body, nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid username or password") {
		t.Errorf("expected error message, got: %s", w.Body.String())
	}
}

// TestWebWrongCredentialsRejected checks that wrong credentials return the error message.
func TestWebWrongCredentialsRejected(t *testing.T) {
	srv, _ := newTestServerWithViewer(t)
	body := url.Values{"username": {"admin"}, "password": {"wrong"}}.Encode()
	w := webRequest(t, srv, "POST", "/login", body, nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid username or password") {
		t.Errorf("expected error message, got: %s", w.Body.String())
	}

	body = url.Values{"username": {"viewer"}, "password": {"wrong"}}.Encode()
	w = webRequest(t, srv, "POST", "/login", body, nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (re-render with error), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid username or password") {
		t.Errorf("expected error message, got: %s", w.Body.String())
	}
}

// TestWebViewerLogout checks that GET /logout clears a viewer session and redirects to /login.
func TestWebViewerLogout(t *testing.T) {
	srv, _ := newTestServerWithViewer(t)
	cookie := loginWebAsViewer(t, srv)

	w := webRequest(t, srv, "GET", "/logout", "", cookie)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 on logout, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}

	// After logout, the old cookie should no longer grant access.
	w = webRequest(t, srv, "GET", "/", "", cookie)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect after logout, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login after logout, got %s", loc)
	}
}

// TestViewerCanAccessReadOnlyRoutes checks that viewer sessions can access read-only routes.
func TestViewerCanAccessReadOnlyRoutes(t *testing.T) {
	srv, _ := newTestServerWithViewer(t)
	cookie := loginWebAsViewer(t, srv)

	// /events is a streaming SSE endpoint and is not suitable for httptest; test / and /data instead.
	readOnlyPaths := []string{"/", "/data"}
	for _, path := range readOnlyPaths {
		w := webRequest(t, srv, "GET", path, "", cookie)
		if w.Code != http.StatusOK {
			t.Errorf("viewer GET %s: expected 200, got %d", path, w.Code)
		}
	}
}

// TestViewerBlockedFromMutatingRoutes checks that viewer sessions get 403 on all mutating POST routes.
func TestViewerBlockedFromMutatingRoutes(t *testing.T) {
	srv, st := newTestServerWithViewer(t)
	cookie := loginWebAsViewer(t, srv)

	_, err := st.CreateProject("viewer-block-project", "tok-viewer-block")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("viewer-block-project", "Viewer Block Feature", "Desc.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	fid := f.ID
	pname := "viewer-block-project"

	mutatingRoutes := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/projects", "name=new-proj&token=tok-new"},
		{"POST", "/project/" + pname + "/features", "name=feat&description=desc"},
		{"POST", "/project/" + pname + "/feature/" + fid + "/description", "description=new"},
		{"POST", "/project/" + pname + "/feature/" + fid + "/start-dialog", ""},
		{"POST", "/project/" + pname + "/feature/" + fid + "/respond", "response=resp"},
		{"POST", "/project/" + pname + "/feature/" + fid + "/reopen", "comment=c"},
		{"POST", "/project/" + pname + "/feature/" + fid + "/generate-now", ""},
		{"POST", "/project/" + pname + "/feature/" + fid + "/generate-after", ""},
		{"POST", "/project/" + pname + "/feature/" + fid + "/rename", "name=newname"},
		{"POST", "/project/" + pname + "/feature/" + fid + "/delete", ""},
	}

	for _, route := range mutatingRoutes {
		w := webRequest(t, srv, route.method, route.path, route.body, cookie)
		if w.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s: expected 403, got %d", route.method, route.path, w.Code)
		}
	}
}

// TestAdminNotBlockedFromMutatingRoutes checks that admin sessions are not blocked by requireAdminMiddleware.
func TestAdminNotBlockedFromMutatingRoutes(t *testing.T) {
	srv, st := newTestServerWithViewer(t)
	cookie := loginWeb(t, srv)

	_, err := st.CreateProject("admin-mutate-project", "tok-admin-mutate")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("admin-mutate-project", "Admin Mutate Feature", "Desc.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Test a representative mutating route; admin should not get 403.
	w := webRequest(t, srv, "POST", "/project/admin-mutate-project/feature/"+f.ID+"/description", "description=updated", cookie)
	if w.Code == http.StatusForbidden {
		t.Errorf("admin should not get 403 on mutating route, got 403")
	}
}

// TestNoSessionBlockedFromMutatingRoutes checks that unauthenticated requests to mutating routes get 302.
func TestNoSessionBlockedFromMutatingRoutes(t *testing.T) {
	srv, st := newTestServer(t)

	_, err := st.CreateProject("nosess-project", "tok-nosess")
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.CreateFeature("nosess-project", "No Session Feature", "Desc.", false, "")
	if err != nil {
		t.Fatal(err)
	}

	fid := f.ID
	pname := "nosess-project"

	mutatingRoutes := []struct {
		path string
		body string
	}{
		{"/projects", "name=new-proj&token=tok-new"},
		{"/project/" + pname + "/features", "name=feat&description=desc"},
		{"/project/" + pname + "/feature/" + fid + "/description", "description=new"},
		{"/project/" + pname + "/feature/" + fid + "/start-dialog", ""},
		{"/project/" + pname + "/feature/" + fid + "/respond", "response=resp"},
		{"/project/" + pname + "/feature/" + fid + "/reopen", "comment=c"},
		{"/project/" + pname + "/feature/" + fid + "/generate-now", ""},
		{"/project/" + pname + "/feature/" + fid + "/generate-after", ""},
		{"/project/" + pname + "/feature/" + fid + "/rename", "name=newname"},
		{"/project/" + pname + "/feature/" + fid + "/delete", ""},
	}

	for _, route := range mutatingRoutes {
		w := webRequest(t, srv, "POST", route.path, route.body, nil)
		if w.Code != http.StatusFound {
			t.Errorf("no session POST %s: expected 302 redirect, got %d", route.path, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "/login" {
			t.Errorf("no session POST %s: expected redirect to /login, got %s", route.path, loc)
		}
	}
}
