package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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

// TestWebProjectView checks that the project view page loads.
func TestWebProjectView(t *testing.T) {
	srv, st := newTestServer(t)
	cookie := loginWeb(t, srv)

	// Create project via store directly.
	_, err := st.CreateProject("view-project", "test-token-abc")
	if err != nil {
		t.Fatal(err)
	}

	w := webRequest(t, srv, "GET", "/project/view-project", "", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "view-project") {
		t.Errorf("expected project name in page")
	}
}

// TestWebProjectViewNotFound checks that a missing project returns 404.
func TestWebProjectViewNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := loginWeb(t, srv)

	w := webRequest(t, srv, "GET", "/project/nonexistent", "", cookie)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
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
	if loc := w.Header().Get("Location"); loc != "/project/create-feat-project" {
		t.Errorf("expected redirect to project page, got: %s", loc)
	}

	// Verify feature appears in project view.
	w2 := webRequest(t, srv, "GET", "/project/create-feat-project", "", cookie)
	if !strings.Contains(w2.Body.String(), "My New Feature") {
		t.Errorf("expected feature name in project view")
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
