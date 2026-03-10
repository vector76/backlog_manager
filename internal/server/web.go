package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yuin/goldmark"

	"github.com/vector76/backlog_manager/internal/model"
)

//go:embed templates static
var webFS embed.FS

// templateFuncs provides helper functions available in all templates.
var templateFuncs = template.FuncMap{
	"beadPercent": func(closed, total int) int {
		if total == 0 {
			return 0
		}
		return closed * 100 / total
	},
	"renderMarkdown": func(md string) template.HTML {
		var buf bytes.Buffer
		if err := goldmark.Convert([]byte(md), &buf); err != nil {
			return template.HTML(template.HTMLEscapeString(md))
		}
		return template.HTML(buf.String())
	},
	"statusBadgeClass": func(status string) string {
		switch status {
		case "awaiting_human":
			return "badge-awaiting-human"
		case "awaiting_client":
			return "badge-awaiting-client"
		case "draft":
			return "badge-draft"
		case "fully_specified":
			return "badge-fully-specified"
		case "done":
			return "badge-done"
		default:
			return "badge-default"
		}
	},
	"hasPrefix": strings.HasPrefix,
	"statusLabel": func(status string) string {
		labels := map[string]string{
			"awaiting_human":   "Awaiting Human",
			"awaiting_client":  "Awaiting Client",
			"draft":            "Draft",
			"fully_specified":  "Fully Specified",
			"waiting":          "Waiting",
			"ready_to_generate": "Ready to Generate",
			"generating":       "Generating",
			"beads_created":    "Beads Created",
			"done":             "Done",
			"halted":           "Halted",
			"abandoned":        "Abandoned",
		}
		if l, ok := labels[status]; ok {
			return l
		}
		return status
	},
}

// mustParseTemplate parses base.html and the given page template together.
// Callers should use t.Execute(w, data) to render the page.
func mustParseTemplate(page string) *template.Template {
	t := template.New("base").Funcs(templateFuncs)
	t = template.Must(t.ParseFS(webFS, "templates/base.html", "templates/"+page+".html"))
	return t
}

// breadcrumb represents a single breadcrumb navigation item.
type breadcrumb struct {
	Label string
	URL   string
}

// basePageData holds fields required by the base template (embedded in all page data structs).
type basePageData struct {
	Breadcrumbs []breadcrumb
	IsViewer    bool
}


// --- Login handlers ---

type loginPageData struct {
	basePageData
	Error string
}

func handleWebLogin(sessions *sessionStore, dashUser, dashPass, viewerUser, viewerPass string) http.HandlerFunc {
	tmpl := mustParseTemplate("login")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				tmpl.Execute(w, loginPageData{Error: "invalid form data"})
				return
			}
			u := r.FormValue("username")
			p := r.FormValue("password")
			var role Role
			if u == dashUser && p == dashPass {
				role = RoleAdmin
			} else if viewerUser != "" && viewerPass != "" && u == viewerUser && p == viewerPass {
				role = RoleViewer
			} else {
				tmpl.Execute(w, loginPageData{Error: "invalid username or password"})
				return
			}
			id, err := sessions.create(role)
			if err != nil {
				tmpl.Execute(w, loginPageData{Error: "internal error"})
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    id,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		// If already logged in, redirect to dashboard.
		if _, ok := sessions.fromRequest(r); ok {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		tmpl.Execute(w, loginPageData{})
	}
}

func handleWebLogout(sessions *sessionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			sessions.delete(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:   sessionCookieName,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// --- Dashboard handler ---

type statCount struct {
	Status string
	Count  int
}

type projectDashData struct {
	Name         string           `json:"name"`
	FeatureCount int              `json:"feature_count"`
	Connectivity string           `json:"connectivity"`
	JustCreated  bool             `json:"just_created"`
	Features     []featureRowData `json:"features"`
}

type newProjectInfo struct {
	Name  string
	Token string
}

type dashboardPageData struct {
	basePageData
	Projects   []projectDashData
	NewProject *newProjectInfo
}

// terminalStatuses is the set of statuses considered terminal (complete/ended).
var terminalStatuses = map[model.FeatureStatus]bool{
	model.StatusDone:      true,
	model.StatusHalted:    true,
	model.StatusAbandoned: true,
}

// buildDashboardData assembles projectDashData for all projects.
func buildDashboardData(st Store, monitor *BeadMonitor) []projectDashData {
	projects := st.ListProjects()
	dashProjects := make([]projectDashData, 0, len(projects))
	for _, p := range projects {
		features, _ := st.ListFeatures(p.Name, nil)
		lastPoll, _ := st.GetLastPollTime(p.Name)

		var active, terminal []model.Feature
		for _, f := range features {
			if terminalStatuses[f.Status] {
				terminal = append(terminal, f)
			} else {
				active = append(active, f)
			}
		}

		sort.Slice(active, func(i, j int) bool {
			return active[i].CreatedAt.After(active[j].CreatedAt)
		})
		sort.Slice(terminal, func(i, j int) bool {
			return terminal[i].CreatedAt.After(terminal[j].CreatedAt)
		})

		allFeatures := append(active, terminal...)
		rows := make([]featureRowData, 0, len(allFeatures))
		for _, f := range allFeatures {
			row := featureRowData{
				ID:           f.ID,
				Name:         f.Name,
				Status:       f.Status.String(),
				UpdatedAt:    f.UpdatedAt.Format("2006-01-02 15:04 UTC"),
				UpdatedAtISO: f.UpdatedAt.Format(time.RFC3339),
			}
			if f.Status == model.StatusBeadsCreated && monitor != nil {
				row.BeadInfo = beadInfoString(f.ID, monitor)
			}
			rows = append(rows, row)
		}

		dashProjects = append(dashProjects, projectDashData{
			Name:         p.Name,
			FeatureCount: len(features),
			Connectivity: connectivityStatus(lastPoll),
			Features:     rows,
		})
	}
	return dashProjects
}

func handleWebDashboard(st Store, monitor *BeadMonitor) http.HandlerFunc {
	tmpl := mustParseTemplate("dashboard")
	return func(w http.ResponseWriter, r *http.Request) {
		// Check for newly created project in query params.
		var newProj *newProjectInfo
		if name := r.URL.Query().Get("created"); name != "" {
			token := r.URL.Query().Get("token")
			if token != "" {
				newProj = &newProjectInfo{Name: name, Token: token}
			}
		}

		dashProjects := buildDashboardData(st, monitor)
		// Mark the just-created project.
		if newProj != nil {
			for i := range dashProjects {
				if dashProjects[i].Name == newProj.Name {
					dashProjects[i].JustCreated = true
					break
				}
			}
		}

		tmpl.Execute(w, dashboardPageData{
			basePageData: basePageData{IsViewer: roleFromContext(r.Context()) == RoleViewer},
			Projects:     dashProjects,
			NewProject:   newProj,
		})
	}
}

// handleWebCreateProject handles POST /projects from the web form.
func handleWebCreateProject(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" || !validProjectName.MatchString(name) {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		token, err := generateToken()
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if _, err := st.CreateProject(name, token); err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/?created="+name+"&token="+token, http.StatusFound)
	}
}

// --- Feature handlers ---

type featureRowData struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	UpdatedAt    string `json:"updated_at"`
	UpdatedAtISO string `json:"updated_at_iso"`
	BeadInfo     string `json:"bead_info,omitempty"` // e.g. "3/7 beads closed" for beads_created features; empty otherwise
}

// beadInfoString returns a human-readable progress string for a feature's beads.
func beadInfoString(featureID string, monitor *BeadMonitor) string {
	if monitor == nil {
		return ""
	}
	p, ok := monitor.GetProgress(featureID)
	if !ok {
		return ""
	}
	if p.Unavailable {
		return "Bead status unavailable"
	}
	return fmt.Sprintf("%d/%d beads closed", p.Closed, p.Total)
}

// --- New feature handler ---

type newFeaturePageData struct {
	basePageData
	ProjectName string
}

func handleWebNewFeature(st Store) http.HandlerFunc {
	tmpl := mustParseTemplate("new_feature")
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		if _, err := st.GetProject(projectName); err != nil {
			http.NotFound(w, r)
			return
		}
		tmpl.Execute(w, newFeaturePageData{
			basePageData: basePageData{
				Breadcrumbs: []breadcrumb{
					{Label: "Dashboard", URL: "/"},
					{Label: projectName},
					{Label: "New Feature"},
				},
				IsViewer: roleFromContext(r.Context()) == RoleViewer,
			},
			ProjectName: projectName,
		})
	}
}

// handleWebCreateFeature handles POST /project/{name}/features from the web form.
func handleWebCreateFeature(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		description := r.FormValue("description")
		directToBead := r.FormValue("direct_to_bead") != ""
		if name == "" {
			http.Redirect(w, r, "/project/"+projectName+"/new", http.StatusFound)
			return
		}
		feat, err := st.CreateFeature(projectName, name, description, directToBead, "")
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		hub.NotifyDashboard()
		http.Redirect(w, r, "/project/"+projectName+"/feature/"+feat.ID, http.StatusFound)
	}
}

// --- Feature detail handler ---

// featureIterationPageData holds data for one dialog iteration on the feature page.
type featureIterationPageData struct {
	Round       int    `json:"round"`
	Description string `json:"description"`
	Questions   string `json:"questions"`
	Response    string `json:"response"`
	IsFinal     bool   `json:"is_final"`
	IsLast      bool   `json:"is_last"`
}

type featureDetailPageData struct {
	basePageData
	ProjectName        string
	Feature            featureRowData
	DirectToBead       bool
	InitialDescription string
	CurrentDescription string
	LatestQuestions    string
	Iterations         []featureIterationPageData
	OtherFeatures      []featureRowData
	BeadProgress       *BeadProgress // non-nil for beads_created features when monitor is available
}

// buildFeatureDetailData assembles featureDetailPageData for a single feature.
func buildFeatureDetailData(st Store, monitor *BeadMonitor, projectName, featureID string) (*featureDetailPageData, error) {
	detail, err := st.GetFeatureDetail(projectName, featureID)
	if err != nil {
		return nil, err
	}

	// Build IsFinal map from feature metadata.
	isFinalMap := make(map[int]bool)
	for _, it := range detail.Feature.Iterations {
		if it.IsFinal {
			isFinalMap[it.Round] = true
		}
	}

	// Build iteration page data with IsLast marked on the final element.
	iterations := make([]featureIterationPageData, len(detail.Iterations))
	for i, ic := range detail.Iterations {
		iterations[i] = featureIterationPageData{
			Round:       ic.Round,
			Description: ic.Description,
			Questions:   ic.Questions,
			Response:    ic.Response,
			IsFinal:     isFinalMap[ic.Round],
			IsLast:      i == len(detail.Iterations)-1,
		}
	}

	// Current description is the most recent non-empty iteration description, or the initial if none.
	currentDesc := detail.InitialDescription
	for i := len(detail.Iterations) - 1; i >= 0; i-- {
		if d := detail.Iterations[i].Description; d != "" {
			currentDesc = d
			break
		}
	}

	// Latest questions are shown prominently when awaiting human response.
	var latestQuestions string
	if detail.Status == model.StatusAwaitingHuman && len(detail.Iterations) > 0 {
		latestQuestions = detail.Iterations[len(detail.Iterations)-1].Questions
	}

	// Build list of incomplete features for the Generate After dropdown.
	var otherFeatures []featureRowData
	if detail.Status == model.StatusFullySpecified {
		if allFeatures, err := st.ListFeatures(projectName, nil); err == nil {
			for _, f := range allFeatures {
				if f.ID != featureID &&
					(f.Status == model.StatusFullySpecified ||
						f.Status == model.StatusWaiting ||
						f.Status == model.StatusReadyToGenerate ||
						f.Status == model.StatusGenerating ||
						f.Status == model.StatusBeadsCreated) {
					otherFeatures = append(otherFeatures, featureRowData{
						ID:     f.ID,
						Name:   f.Name,
						Status: f.Status.String(),
					})
				}
			}
		}
	}

	// Bead progress for features in beads_created status.
	var beadProgress *BeadProgress
	if detail.Status == model.StatusBeadsCreated && monitor != nil {
		if p, ok := monitor.GetProgress(detail.ID); ok {
			beadProgress = &p
		}
	}

	return &featureDetailPageData{
		basePageData: basePageData{Breadcrumbs: []breadcrumb{
			{Label: "Dashboard", URL: "/"},
			{Label: projectName},
			{Label: detail.Name},
		}},
		ProjectName: projectName,
		Feature: featureRowData{
			ID:           detail.ID,
			Name:         detail.Name,
			Status:       detail.Status.String(),
			UpdatedAt:    detail.UpdatedAt.Format("2006-01-02 15:04 UTC"),
			UpdatedAtISO: detail.UpdatedAt.Format(time.RFC3339),
		},
		DirectToBead:       detail.DirectToBead,
		InitialDescription: detail.InitialDescription,
		CurrentDescription: currentDesc,
		LatestQuestions:    latestQuestions,
		Iterations:         iterations,
		OtherFeatures:      otherFeatures,
		BeadProgress:       beadProgress,
	}, nil
}

func handleWebFeature(st Store, monitor *BeadMonitor) http.HandlerFunc {
	tmpl := mustParseTemplate("feature")
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		data, err := buildFeatureDetailData(st, monitor, projectName, featureID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data.IsViewer = roleFromContext(r.Context()) == RoleViewer
		tmpl.Execute(w, data)
	}
}

// handleWebUpdateDescription handles POST /project/{name}/feature/{id}/description.
// It updates description_v0.md for a draft feature and redirects back.
func handleWebUpdateDescription(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		featurePage := "/project/" + projectName + "/feature/" + featureID
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		description := r.FormValue("description")
		_ = st.UpdateDescriptionV0(projectName, featureID, description)
		hub.NotifyFeature(projectName + ":" + featureID)
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}

// handleWebStartDialog handles POST /project/{name}/feature/{id}/start-dialog.
// It starts the dialog for a draft feature and redirects back.
func handleWebStartDialog(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		_ = st.StartDialog(projectName, featureID)
		hub.NotifyFeature(projectName + ":" + featureID)
		http.Redirect(w, r, "/project/"+projectName+"/feature/"+featureID, http.StatusFound)
	}
}

// handleWebRespond handles POST /project/{name}/feature/{id}/respond.
// It stores the user's response and redirects back. The "final" form field controls
// whether this is marked as a final response.
func handleWebRespond(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		featurePage := "/project/" + projectName + "/feature/" + featureID
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		response := r.FormValue("response")
		final := r.FormValue("final") == "on"
		_ = st.RespondToDialog(projectName, featureID, response, final)
		hub.NotifyFeature(projectName + ":" + featureID)
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}

// handleWebReopen handles POST /project/{name}/feature/{id}/reopen.
// It reopens a fully-specified feature dialog and redirects back.
func handleWebReopen(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		featurePage := "/project/" + projectName + "/feature/" + featureID
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		message := r.FormValue("message")
		_ = st.ReopenDialog(projectName, featureID, message)
		hub.NotifyFeature(projectName + ":" + featureID)
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}

// handleWebRenameFeature handles POST /project/{name}/feature/{id}/rename.
// Updates the feature name and redirects back to the feature page.
func handleWebRenameFeature(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		featurePage := "/project/" + projectName + "/feature/" + featureID
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		f.Name = name
		if err := st.UpdateFeature(f); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		hub.NotifyFeature(projectName + ":" + featureID)
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}

// handleWebDeleteDraftFeature handles POST /project/{name}/feature/{id}/delete.
// Hard-deletes a draft feature and redirects to the dashboard.
// Non-draft features are rejected with a redirect back to the feature page.
func handleWebDeleteDraftFeature(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		featurePage := "/project/" + projectName + "/feature/" + featureID
		f, err := st.GetFeature(projectName, featureID)
		if err != nil || f.Status != model.StatusDraft {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		if err := st.DeleteFeature(projectName, featureID); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// handleWebGenerateNow handles POST /project/{name}/feature/{id}/generate-now.
// Transitions a fully_specified feature to ready_to_generate and redirects back.
func handleWebGenerateNow(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		_ = st.TransitionStatus(projectName, featureID, model.StatusReadyToGenerate)
		hub.NotifyFeature(projectName + ":" + featureID)
		http.Redirect(w, r, "/project/"+projectName+"/feature/"+featureID, http.StatusFound)
	}
}

// handleWebGenerateAfter handles POST /project/{name}/feature/{id}/generate-after.
// Sets a dependency on another feature and transitions to waiting, then redirects back.
func handleWebGenerateAfter(st Store, hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		featurePage := "/project/" + projectName + "/feature/" + featureID
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		afterFeatureID := r.FormValue("after_feature_id")
		if afterFeatureID == "" {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		f, err := st.GetFeature(projectName, featureID)
		if err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		if f.Status != model.StatusFullySpecified {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		f.GenerateAfter = afterFeatureID
		_ = st.UpdateFeature(f)
		_ = st.TransitionStatus(projectName, featureID, model.StatusWaiting)
		hub.NotifyFeature(projectName + ":" + featureID)
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}
