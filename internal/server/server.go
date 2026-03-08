// Package server provides the HTTP server and chi router for the backlog manager.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/vector76/backlog_manager/internal/config"
	"github.com/vector76/backlog_manager/internal/model"
	"github.com/vector76/backlog_manager/internal/store"
)

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
}

type contextKey string

const projectContextKey contextKey = "project"

// New creates a new HTTP server with the given config and store.
func New(cfg *config.Config, st Store) *http.Server {
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
	})

	// Token auth routes
	r.Group(func(r chi.Router) {
		r.Use(tokenAuthMiddleware(st))
		r.Get("/api/v1/project", handleGetOwnProject(st))
	})

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}
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
			resp = append(resp, projectResponse{
				Name:         p.Name,
				FeatureCount: len(features),
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
		writeJSON(w, http.StatusOK, projectResponse{
			Name:         project.Name,
			FeatureCount: len(features),
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
		writeJSON(w, http.StatusOK, projectResponse{
			Name:         project.Name,
			FeatureCount: len(features),
		})
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
