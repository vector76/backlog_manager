package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to dashboard, got: %s", loc)
	}

	// Verify feature appears on dashboard.
	w2 := webRequest(t, srv, "GET", "/", "", cookie)
	if !strings.Contains(w2.Body.String(), "My New Feature") {
		t.Errorf("expected feature name in dashboard")
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
	f, err := st.CreateFeature("md-project", "MD Feature", "## Overview\n\nHello **world**.")
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
	f, err := st.CreateFeature("act-project", "Draft Feature", "Initial description here.")
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
	f, err := st.CreateFeature("upd-project", "Upd Feature", "Old description.")
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
	f, err := st.CreateFeature("dialog-project", "Dialog Feature", "Some description.")
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
	f, err := st.CreateFeature("ah-project", "AH Feature", "Description.")
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
	f, err := st.CreateFeature("resp-project", "Resp Feature", "Description.")
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
	f, err := st.CreateFeature("final-project", "Final Feature", "Description.")
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
	f, err := st.CreateFeature("fs-project", "FS Feature", "Description.")
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
	f, err := st.CreateFeature("reopen-project", "Reopen Feature", "Description.")
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
	f, err := st.CreateFeature("reopen-desc-project", "Desc Feature", "Original description.")
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
	f, err := st.CreateFeature("hist-project", "History Feature", "Initial.")
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
	f, err := st.CreateFeature("wait-project", "Wait Feature", "Description.")
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
