// Package server provides the HTTP server and chi router for the backlog manager.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/vector76/backlog_manager/internal/config"
	"github.com/vector76/backlog_manager/internal/model"
	"github.com/vector76/backlog_manager/internal/store"
)

// pollTimeout is the maximum duration a poll request will block.
const pollTimeout = 30 * time.Second

// Version is the server version string.
const Version = "0.1.0"

// Store is the interface the server needs from the store.
type Store interface {
	CreateProject(name, token string) (*model.Project, error)
	ListProjects() []model.Project
	GetProject(name string) (*model.Project, error)
	DeleteProject(name string) error
	GetProjectByToken(token string) (*model.Project, error)
	ListFeatures(projectName string, statusFilter *model.FeatureStatus) ([]model.Feature, error)
	CreateFeature(projectName, featureName, description string, directToBead bool, generateAfter string) (*model.Feature, error)
	GetFeature(projectName, featureID string) (*model.Feature, error)
	GetFeatureDetail(projectName, featureID string) (*model.FeatureDetail, error)
	UpdateFeature(updated *model.Feature) error
	AppendBeadID(projectName, featureID, beadID string) error
	UpdateDescriptionV0(projectName, featureID, description string) error
	StartDialog(projectName, featureID string) error
	RespondToDialog(projectName, featureID string, response string, final bool) error
	ReopenDialog(projectName, featureID string, message string) error
	RecordPoll(projectName string)
	GetLastPollTime(projectName string) (time.Time, bool)
	SubmitClientDialog(projectName, featureID, updatedDescription, questions string) error
	ReadDescriptionVersion(projectName, featureID string, version int) (string, error)
	ReadQuestions(projectName, featureID string, round int) (string, error)
	ReadResponse(projectName, featureID string, round int) (string, error)
	DeleteFeature(projectName, featureID string) error
	TransitionStatus(projectName, featureID string, newStatus model.FeatureStatus) error
	WriteArtifact(projectName, featureID, name, content string) error
}

type contextKey string

const projectContextKey contextKey = "project"

// New creates a new HTTP server with the given config and store.
// An optional BeadMonitor may be provided to enable bead status polling.
// Returns the HTTP server and the NotifyHub for broadcasting SSE events.
func New(cfg *config.Config, st Store, monitors ...*BeadMonitor) (*http.Server, *NotifyHub) {
	var monitor *BeadMonitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}

	hub := NewNotifyHub()
	hub.Start(context.Background())

	if monitor != nil {
		monitor.SetNotify(func(proj, feat string) { hub.NotifyFeature(proj + ":" + feat) })
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/api/v1/health", handleHealth)
	r.Get("/api/v1/version", handleVersion)

	// Dashboard auth routes
	r.Group(func(r chi.Router) {
		r.Use(dashboardAuthMiddleware(cfg.DashboardUser, cfg.DashboardPassword))
		r.Post("/api/v1/projects", handleCreateProject(st))
		r.Get("/api/v1/projects", handleListProjects(st))
		r.Get("/api/v1/projects/{name}", handleGetProject(st))
		r.Delete("/api/v1/projects/{name}", handleDeleteProject(st))
		r.Post("/api/v1/projects/{name}/features", handleCreateFeature(st, hub))
		r.Get("/api/v1/projects/{name}/features", handleListProjectFeatures(st))
		r.Get("/api/v1/projects/{name}/features/{id}", handleGetProjectFeature(st))
		r.Patch("/api/v1/projects/{name}/features/{id}", handleUpdateFeature(st, hub))
		r.Delete("/api/v1/projects/{name}/features/{id}", handleAbandonFeature(st, hub))
		r.Post("/api/v1/projects/{name}/features/{id}/start-dialog", handleStartDialog(st, hub))
		r.Post("/api/v1/projects/{name}/features/{id}/respond", handleRespondToDialog(st, hub))
		r.Post("/api/v1/projects/{name}/features/{id}/reopen", handleReopenDialog(st, hub))
		r.Post("/api/v1/projects/{name}/features/{id}/generate-now", handleGenerateNow(st, hub))
		r.Post("/api/v1/projects/{name}/features/{id}/generate-after", handleGenerateAfter(st, hub))
	})

	// Token auth routes
	r.Group(func(r chi.Router) {
		r.Use(tokenAuthMiddleware(st))
		r.Get("/api/v1/project", handleGetOwnProject(st))
		r.Get("/api/v1/features", handleListClientFeatures(st))
		r.Get("/api/v1/features/{id}", handleGetClientFeature(st))
		r.Get("/api/v1/poll", handlePoll(st, hub))
		r.Get("/api/v1/features/{id}/pending", handleGetPending(st))
		r.Post("/api/v1/features/{id}/submit-dialog", handleSubmitDialog(st, hub))
		r.Post("/api/v1/features/{id}/start-generate", handleStartGenerate(st, hub))
		r.Post("/api/v1/features/{id}/register-bead", handleRegisterBead(st, hub))
		r.Post("/api/v1/features/{id}/beads-done", handleBeadsDone(st, hub))
		r.Post("/api/v1/features/{id}/register-artifact", handleRegisterArtifact(st, hub))
		r.Post("/api/v1/features/{id}/complete", handleCompleteFeature(st, hub))
	})

	// Web dashboard routes
	sessions := newSessionStore()
	staticFS, _ := fs.Sub(webFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static", http.FileServer(http.FS(staticFS))))
	loginHandler := handleWebLogin(sessions, cfg.DashboardUser, cfg.DashboardPassword)
	r.Get("/login", loginHandler)
	r.Post("/login", loginHandler)
	r.Get("/logout", handleWebLogout(sessions))
	r.Group(func(r chi.Router) {
		r.Use(webSessionMiddleware(sessions))
		r.Get("/", handleWebDashboard(st, monitor))
		r.Post("/projects", handleWebCreateProject(st))
		r.Get("/project/{name}/new", handleWebNewFeature(st))
		r.Post("/project/{name}/features", handleWebCreateFeature(st, hub))
		r.Get("/project/{name}/feature/{id}", handleWebFeature(st, monitor))
		r.Post("/project/{name}/feature/{id}/description", handleWebUpdateDescription(st, hub))
		r.Post("/project/{name}/feature/{id}/start-dialog", handleWebStartDialog(st, hub))
		r.Post("/project/{name}/feature/{id}/respond", handleWebRespond(st, hub))
		r.Post("/project/{name}/feature/{id}/reopen", handleWebReopen(st, hub))
		r.Post("/project/{name}/feature/{id}/generate-now", handleWebGenerateNow(st, hub))
		r.Post("/project/{name}/feature/{id}/generate-after", handleWebGenerateAfter(st, hub))
		r.Post("/project/{name}/feature/{id}/delete", handleWebDeleteDraftFeature(st))
		r.Get("/events", handleDashboardSSE(hub))
		r.Get("/data", handleDashboardData(st, monitor))
		r.Get("/project/{name}/feature/{id}/events", handleFeatureSSE(hub))
		r.Get("/project/{name}/feature/{id}/data", handleFeatureData(st, monitor))
	})

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}, hub
}

// dashboardAuthMiddleware validates HTTP Basic Auth credentials.
func dashboardAuthMiddleware(user, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || u != user || p != password {
				w.Header().Set("WWW-Authenticate", `Basic realm="backlog-manager"`)
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tokenAuthMiddleware validates Bearer token auth and injects the project into context.
func tokenAuthMiddleware(st Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			project, err := st.GetProjectByToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), projectContextKey, project)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// --- Response helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Handlers ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": Version})
}

// validProjectName matches safe project names: alphanumeric, hyphens, underscores; must start with a letter or digit.
var validProjectName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// createProjectRequest is the request body for POST /api/v1/projects.
type createProjectRequest struct {
	Name string `json:"name"`
}

// projectResponse is the response for project API endpoints.
type projectResponse struct {
	Name         string `json:"name"`
	Token        string `json:"token,omitempty"`
	FeatureCount int    `json:"feature_count"`
	Connectivity string `json:"connectivity,omitempty"`
}

// connectivityStatus returns a human-readable connectivity status string based on last poll time.
func connectivityStatus(lastPoll time.Time) string {
	if lastPoll.IsZero() {
		return ""
	}
	elapsed := time.Since(lastPoll)
	if elapsed <= 2*pollTimeout {
		return "Connected (<1 min)"
	}
	mins := int((elapsed + 30*time.Second) / time.Minute)
	return fmt.Sprintf("Last seen: %d min ago", mins)
}

func handleCreateProject(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if !validProjectName.MatchString(req.Name) {
			writeError(w, http.StatusBadRequest, "name must contain only letters, digits, hyphens, and underscores")
			return
		}

		token, err := generateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		project, err := st.CreateProject(req.Name, token)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		writeJSON(w, http.StatusCreated, projectResponse{
			Name:         project.Name,
			Token:        project.Token,
			FeatureCount: 0,
		})
	}
}

func handleListProjects(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects := st.ListProjects()
		resp := make([]projectResponse, 0, len(projects))
		for _, p := range projects {
			features, _ := st.ListFeatures(p.Name, nil)
			lastPoll, _ := st.GetLastPollTime(p.Name)
			resp = append(resp, projectResponse{
				Name:         p.Name,
				FeatureCount: len(features),
				Connectivity: connectivityStatus(lastPoll),
			})
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleGetProject(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		project, err := st.GetProject(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		features, _ := st.ListFeatures(project.Name, nil)
		lastPoll, _ := st.GetLastPollTime(project.Name)
		writeJSON(w, http.StatusOK, projectResponse{
			Name:         project.Name,
			FeatureCount: len(features),
			Connectivity: connectivityStatus(lastPoll),
		})
	}
}

func handleDeleteProject(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if err := st.DeleteProject(name); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleGetOwnProject(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		features, _ := st.ListFeatures(project.Name, nil)
		lastPoll, _ := st.GetLastPollTime(project.Name)
		writeJSON(w, http.StatusOK, projectResponse{
			Name:         project.Name,
			FeatureCount: len(features),
			Connectivity: connectivityStatus(lastPoll),
		})
	}
}

// --- Feature request/response types ---

type createFeatureRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	DirectToBead  bool   `json:"direct_to_bead"`
	GenerateAfter string `json:"generate_after"`
}

type updateFeatureRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// featureResponse is the JSON representation of a feature for API responses.
type featureResponse struct {
	ID               string   `json:"id"`
	Project          string   `json:"project"`
	Name             string   `json:"name"`
	Status           string   `json:"status"`
	CurrentIteration int      `json:"current_iteration"`
	DirectToBead     bool     `json:"direct_to_bead,omitempty"`
	GenerateAfter    string   `json:"generate_after,omitempty"`
	BeadIDs          []string `json:"bead_ids,omitempty"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

// featureDetailResponse extends featureResponse with description content and history.
type featureDetailResponse struct {
	featureResponse
	InitialDescription string                   `json:"initial_description"`
	Iterations         []model.IterationContent `json:"iterations,omitempty"`
}

func toFeatureResponse(f model.Feature) featureResponse {
	return featureResponse{
		ID:               f.ID,
		Project:          f.Project,
		Name:             f.Name,
		Status:           f.Status.String(),
		CurrentIteration: f.CurrentIteration,
		DirectToBead:     f.DirectToBead,
		GenerateAfter:    f.GenerateAfter,
		BeadIDs:          f.BeadIDs,
		CreatedAt:        f.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        f.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toFeatureDetailResponse(d *model.FeatureDetail) featureDetailResponse {
	return featureDetailResponse{
		featureResponse:    toFeatureResponse(d.Feature),
		InitialDescription: d.InitialDescription,
		Iterations:         d.Iterations,
	}
}

// parseStatusFilter parses a comma-separated ?status= query param into a slice of FeatureStatus.
// Returns nil if the param is absent or empty (meaning no filter).
func parseStatusFilter(r *http.Request) ([]model.FeatureStatus, error) {
	raw := r.URL.Query().Get("status")
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	statuses := make([]model.FeatureStatus, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		data, _ := json.Marshal(p)
		var fs model.FeatureStatus
		if err := fs.UnmarshalJSON(data); err != nil {
			return nil, fmt.Errorf("unknown status %q", p)
		}
		statuses = append(statuses, fs)
	}
	return statuses, nil
}

func filterByStatuses(features []model.Feature, statuses []model.FeatureStatus) []model.Feature {
	if len(statuses) == 0 {
		return features
	}
	set := make(map[model.FeatureStatus]bool, len(statuses))
	for _, s := range statuses {
		set[s] = true
	}
	var result []model.Feature
	for _, f := range features {
		if set[f.Status] {
			result = append(result, f)
		}
	}
	return result
}

// --- Dashboard feature handlers ---

func handleCreateFeature(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		if _, err := st.GetProject(projectName); err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		var req createFeatureRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		f, err := st.CreateFeature(projectName, req.Name, req.Description, req.DirectToBead, req.GenerateAfter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyDashboard()
		writeJSON(w, http.StatusCreated, toFeatureResponse(*f))
	}
}

func handleListProjectFeatures(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		if _, err := st.GetProject(projectName); err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		statuses, err := parseStatusFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		all, err := st.ListFeatures(projectName, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		filtered := filterByStatuses(all, statuses)
		resp := make([]featureResponse, 0, len(filtered))
		for _, f := range filtered {
			resp = append(resp, toFeatureResponse(f))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleGetProjectFeature(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		detail, err := st.GetFeatureDetail(projectName, featureID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		writeJSON(w, http.StatusOK, toFeatureDetailResponse(detail))
	}
}

func handleUpdateFeature(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		var req updateFeatureRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		// Validate all inputs before performing any writes.
		if req.Name != nil && *req.Name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		if req.Description != nil && f.Status != model.StatusDraft {
			writeError(w, http.StatusConflict, "description can only be updated in draft status")
			return
		}

		if req.Description != nil {
			if err := st.UpdateDescriptionV0(projectName, featureID, *req.Description); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
		if req.Name != nil {
			f.Name = *req.Name
		}
		changed := req.Name != nil || req.Description != nil
		if changed {
			if err := st.UpdateFeature(f); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
		// Re-fetch after updates
		updated, err := st.GetFeature(projectName, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if changed {
			hub.NotifyFeature(projectName + ":" + featureID)
		}
		writeJSON(w, http.StatusOK, toFeatureResponse(*updated))
	}
}

func handleAbandonFeature(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		if f.Status == model.StatusDraft {
			if err := st.DeleteFeature(projectName, featureID); err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		f.Status = model.StatusAbandoned
		if err := st.UpdateFeature(f); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(projectName + ":" + featureID)
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Client feature handlers ---

func handleListClientFeatures(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		statuses, err := parseStatusFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		all, err := st.ListFeatures(project.Name, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		filtered := filterByStatuses(all, statuses)
		resp := make([]featureResponse, 0, len(filtered))
		for _, f := range filtered {
			resp = append(resp, toFeatureResponse(f))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleGetClientFeature(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")
		detail, err := st.GetFeatureDetail(project.Name, featureID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		writeJSON(w, http.StatusOK, toFeatureDetailResponse(detail))
	}
}

// --- Dialog state machine handlers ---

func handleStartDialog(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		if err := st.StartDialog(projectName, featureID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid transition") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(projectName + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*f))
	}
}

type respondRequest struct {
	Response string `json:"response"`
	Final    bool   `json:"final"`
}

func handleRespondToDialog(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		var req respondRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := st.RespondToDialog(projectName, featureID, req.Response, req.Final); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid transition") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(projectName + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*f))
	}
}

type reopenRequest struct {
	Message string `json:"message"`
}

func handleReopenDialog(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		var req reopenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := st.ReopenDialog(projectName, featureID, req.Message); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid transition") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(projectName + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*f))
	}
}

// --- Client dialog handlers ---

// pollResponse is the response for GET /api/v1/poll.
type pollResponse struct {
	Action       string `json:"action"`
	FeatureID    string `json:"feature_id"`
	FeatureName  string `json:"feature_name"`
	DirectToBead bool   `json:"direct_to_bead,omitempty"`
}

// findActionableFeature returns the first feature in awaiting_client or ready_to_generate status.
func findActionableFeature(st Store, projectName string) (model.Feature, model.FeatureAction, bool) {
	features, err := st.ListFeatures(projectName, nil)
	if err != nil {
		return model.Feature{}, 0, false
	}
	for _, f := range features {
		switch f.Status {
		case model.StatusAwaitingClient:
			return f, model.ActionDialogStep, true
		case model.StatusReadyToGenerate:
			return f, model.ActionGenerate, true
		}
	}
	return model.Feature{}, 0, false
}

// handlePoll handles GET /api/v1/poll — long-poll until work is available.
// Returns 200 with action JSON when work is available; 204 on timeout.
// Accepts optional ?timeout=N query param (seconds, max 30) for testing.
func handlePoll(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		st.RecordPoll(project.Name)
		hub.NotifyDashboard()

		timeout := pollTimeout
		if q := r.URL.Query().Get("timeout"); q != "" {
			if secs, err := strconv.Atoi(q); err == nil && secs > 0 && secs <= int(pollTimeout.Seconds()) {
				timeout = time.Duration(secs) * time.Second
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			if feature, action, found := findActionableFeature(st, project.Name); found {
				resp := pollResponse{
					Action:      action.String(),
					FeatureID:   feature.ID,
					FeatureName: feature.Name,
				}
				if action == model.ActionGenerate {
					resp.DirectToBead = feature.DirectToBead
				}
				writeJSON(w, http.StatusOK, resp)
				return
			}

			select {
			case <-ctx.Done():
				w.WriteHeader(http.StatusNoContent)
				return
			case <-ticker.C:
				// Check again
			}
		}
	}
}

// pendingResponse is the response for GET /api/v1/features/{id}/pending.
type pendingResponse struct {
	Iteration          int    `json:"iteration"`
	FeatureDescription string `json:"feature_description"`
	Questions          string `json:"questions"`
	UserResponse       string `json:"user_response"`
	IsFinal            bool   `json:"is_final"`
}

// handleGetPending handles GET /api/v1/features/{id}/pending.
func handleGetPending(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")

		feature, err := st.GetFeature(project.Name, featureID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		if feature.Status != model.StatusAwaitingClient && feature.Status != model.StatusReadyToGenerate {
			writeError(w, http.StatusConflict, "feature is not in an actionable state")
			return
		}

		N := feature.CurrentIteration
		resp := pendingResponse{Iteration: N}

		if feature.Status == model.StatusReadyToGenerate {
			desc, err := st.ReadDescriptionVersion(project.Name, featureID, N)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			resp.FeatureDescription = desc
			writeJSON(w, http.StatusOK, resp)
			return
		}

		// status == awaiting_client
		if N == 0 {
			// First round: return initial description only.
			desc, err := st.ReadDescriptionVersion(project.Name, featureID, 0)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			resp.FeatureDescription = desc
			writeJSON(w, http.StatusOK, resp)
			return
		}

		// N >= 1: check if description_vN exists (subsequent round) or not (reopen).
		desc, err := st.ReadDescriptionVersion(project.Name, featureID, N)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if desc != "" {
			// Subsequent round: client wrote description_vN and questions_vN, human responded.
			questions, err := st.ReadQuestions(project.Name, featureID, N)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			response, err := st.ReadResponse(project.Name, featureID, N)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			resp.FeatureDescription = desc
			resp.Questions = questions
			resp.UserResponse = response
			for _, it := range feature.Iterations {
				if it.Round == N {
					resp.IsFinal = it.IsFinal
					break
				}
			}
		} else {
			// Reopen: response_vN has the reopen message, description is from previous round.
			prevDesc, err := st.ReadDescriptionVersion(project.Name, featureID, N-1)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			response, err := st.ReadResponse(project.Name, featureID, N)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			resp.FeatureDescription = prevDesc
			resp.UserResponse = response
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// submitDialogRequest is the request body for POST /api/v1/features/{id}/submit-dialog.
type submitDialogRequest struct {
	UpdatedDescription string `json:"updated_description"`
	Questions          string `json:"questions"`
}

// handleSubmitDialog handles POST /api/v1/features/{id}/submit-dialog.
func handleSubmitDialog(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")

		var req submitDialogRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.UpdatedDescription == "" {
			writeError(w, http.StatusBadRequest, "updated_description is required")
			return
		}

		if err := st.SubmitClientDialog(project.Name, featureID, req.UpdatedDescription, req.Questions); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid transition") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		f, err := st.GetFeature(project.Name, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(project.Name + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*f))
	}
}

// --- Generation pipeline handlers ---

// handleGenerateNow handles POST /api/v1/projects/{name}/features/{id}/generate-now.
// Transitions a fully_specified feature to ready_to_generate.
func handleGenerateNow(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		if err := st.TransitionStatus(projectName, featureID, model.StatusReadyToGenerate); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(projectName + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*f))
	}
}

type generateAfterRequest struct {
	AfterFeatureID string `json:"after_feature_id"`
}

// handleGenerateAfter handles POST /api/v1/projects/{name}/features/{id}/generate-after.
// Sets a dependency on another feature and transitions to waiting.
func handleGenerateAfter(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		var req generateAfterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.AfterFeatureID == "" {
			writeError(w, http.StatusBadRequest, "after_feature_id is required")
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		if f.Status != model.StatusFullySpecified {
			writeError(w, http.StatusConflict, fmt.Sprintf("generate-after requires fully_specified status, feature is in %v", f.Status))
			return
		}
		f.GenerateAfter = req.AfterFeatureID
		if err := st.UpdateFeature(f); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := st.TransitionStatus(projectName, featureID, model.StatusWaiting); err != nil {
			if strings.Contains(err.Error(), "invalid") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		updated, err := st.GetFeature(projectName, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(projectName + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*updated))
	}
}

// handleStartGenerate handles POST /api/v1/features/{id}/start-generate.
// Transitions a ready_to_generate feature to generating.
func handleStartGenerate(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")
		if err := st.TransitionStatus(project.Name, featureID, model.StatusGenerating); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		f, err := st.GetFeature(project.Name, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(project.Name + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*f))
	}
}

type registerBeadRequest struct {
	BeadID string `json:"bead_id"`
}

// handleRegisterBead handles POST /api/v1/features/{id}/register-bead.
// Appends a single bead ID to the feature; feature remains in generating status.
func handleRegisterBead(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")
		var req registerBeadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.BeadID == "" {
			writeError(w, http.StatusBadRequest, "bead_id is required")
			return
		}
		if err := st.AppendBeadID(project.Name, featureID, req.BeadID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		updated, err := st.GetFeature(project.Name, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(project.Name + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*updated))
	}
}

// handleBeadsDone handles POST /api/v1/features/{id}/beads-done.
// Transitions a generating feature to beads_created; no body required.
func handleBeadsDone(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")
		f, err := st.GetFeature(project.Name, featureID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		if f.Status != model.StatusGenerating {
			writeError(w, http.StatusConflict, fmt.Sprintf("beads-done requires generating status, feature is in %v", f.Status))
			return
		}
		if err := st.TransitionStatus(project.Name, featureID, model.StatusBeadsCreated); err != nil {
			if strings.Contains(err.Error(), "invalid") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		updated, err := st.GetFeature(project.Name, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(project.Name + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*updated))
	}
}

type registerArtifactRequest struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// handleRegisterArtifact handles POST /api/v1/features/{id}/register-artifact.
// Stores a plan.md or beads.md artifact for a feature.
func handleRegisterArtifact(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")
		var req registerArtifactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Type != "plan" && req.Type != "beads" {
			writeError(w, http.StatusBadRequest, "type must be 'plan' or 'beads'")
			return
		}
		if req.Content == "" {
			writeError(w, http.StatusBadRequest, "content is required")
			return
		}
		if err := st.WriteArtifact(project.Name, featureID, req.Type+".md", req.Content); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		st.RecordPoll(project.Name)
		hub.NotifyDashboard()
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleCompleteFeature handles POST /api/v1/features/{id}/complete.
// Transitions beads_created -> done and unblocks dependent waiting features.
func handleCompleteFeature(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project, ok := r.Context().Value(projectContextKey).(*model.Project)
		if !ok || project == nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		featureID := chi.URLParam(r, "id")
		if err := st.TransitionStatus(project.Name, featureID, model.StatusDone); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
			} else if strings.Contains(err.Error(), "invalid") {
				writeError(w, http.StatusConflict, err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		// Dependency resolution: unblock waiting features that depended on this one.
		if features, err := st.ListFeatures(project.Name, nil); err == nil {
			for _, f := range features {
				if f.Status == model.StatusWaiting && f.GenerateAfter == featureID {
					_ = st.TransitionStatus(project.Name, f.ID, model.StatusReadyToGenerate)
				}
			}
		}
		updated, err := st.GetFeature(project.Name, featureID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		hub.NotifyFeature(project.Name + ":" + featureID)
		writeJSON(w, http.StatusOK, toFeatureResponse(*updated))
	}
}

// generateToken creates a secure random hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewStore is a convenience constructor that returns a *store.Store as a Store interface.
func NewStore(dataDir string) (Store, error) {
	return store.New(dataDir)
}
