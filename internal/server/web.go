package server

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yuin/goldmark"

	"github.com/vector76/backlog_manager/internal/model"
)

//go:embed templates static
var webFS embed.FS

// templateFuncs provides helper functions available in all templates.
var templateFuncs = template.FuncMap{
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
}

// statusOrder defines the display order for feature status groups.
var statusOrder = []model.FeatureStatus{
	model.StatusAwaitingHuman,
	model.StatusAwaitingClient,
	model.StatusDraft,
	model.StatusFullySpecified,
	model.StatusWaiting,
	model.StatusReadyToGenerate,
	model.StatusGenerating,
	model.StatusBeadsCreated,
	model.StatusDone,
	model.StatusHalted,
	model.StatusAbandoned,
}

// --- Login handlers ---

type loginPageData struct {
	basePageData
	Error string
}

func handleWebLogin(sessions *sessionStore, dashUser, dashPass string) http.HandlerFunc {
	tmpl := mustParseTemplate("login")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				tmpl.Execute(w, loginPageData{Error: "invalid form data"})
				return
			}
			u := r.FormValue("username")
			p := r.FormValue("password")
			if u != dashUser || p != dashPass {
				tmpl.Execute(w, loginPageData{Error: "invalid username or password"})
				return
			}
			id, err := sessions.create()
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
	Name         string
	FeatureCount int
	Connectivity string
	StatCounts   []statCount
	JustCreated  bool
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

func handleWebDashboard(st Store) http.HandlerFunc {
	tmpl := mustParseTemplate("dashboard")
	return func(w http.ResponseWriter, r *http.Request) {
		projects := st.ListProjects()

		// Check for newly created project in query params.
		var newProj *newProjectInfo
		if name := r.URL.Query().Get("created"); name != "" {
			token := r.URL.Query().Get("token")
			if token != "" {
				newProj = &newProjectInfo{Name: name, Token: token}
			}
		}

		dashProjects := make([]projectDashData, 0, len(projects))
		for _, p := range projects {
			features, _ := st.ListFeatures(p.Name, nil)
			lastPoll, _ := st.GetLastPollTime(p.Name)

			// Count by status.
			counts := make(map[string]int)
			for _, f := range features {
				counts[f.Status.String()]++
			}
			var sc []statCount
			for _, s := range statusOrder {
				key := s.String()
				if n, ok := counts[key]; ok && n > 0 {
					sc = append(sc, statCount{Status: key, Count: n})
				}
			}

			dashProjects = append(dashProjects, projectDashData{
				Name:         p.Name,
				FeatureCount: len(features),
				Connectivity: connectivityStatus(lastPoll),
				StatCounts:   sc,
				JustCreated:  newProj != nil && p.Name == newProj.Name,
			})
		}

		tmpl.Execute(w, dashboardPageData{
			Projects:   dashProjects,
			NewProject: newProj,
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

// --- Project view handler ---

type featureRowData struct {
	ID        string
	Name      string
	Status    string
	UpdatedAt string
}

type featureGroupData struct {
	Status   string
	Features []featureRowData
}

type projectPageData struct {
	basePageData
	ProjectName string
	Groups      []featureGroupData
}

func handleWebProject(st Store) http.HandlerFunc {
	tmpl := mustParseTemplate("project")
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		if _, err := st.GetProject(projectName); err != nil {
			http.NotFound(w, r)
			return
		}
		features, _ := st.ListFeatures(projectName, nil)

		// Group features by status in the defined order.
		byStatus := make(map[model.FeatureStatus][]featureRowData)
		for _, f := range features {
			byStatus[f.Status] = append(byStatus[f.Status], featureRowData{
				ID:        f.ID,
				Name:      f.Name,
				Status:    f.Status.String(),
				UpdatedAt: f.UpdatedAt.Format("2006-01-02 15:04"),
			})
		}

		var groups []featureGroupData
		for _, s := range statusOrder {
			rows, ok := byStatus[s]
			if !ok || len(rows) == 0 {
				continue
			}
			// Sort within group: most recently updated first.
			sort.Slice(rows, func(i, j int) bool {
				return rows[i].UpdatedAt > rows[j].UpdatedAt
			})
			groups = append(groups, featureGroupData{
				Status:   s.String(),
				Features: rows,
			})
		}

		tmpl.Execute(w, projectPageData{
			basePageData: basePageData{Breadcrumbs: []breadcrumb{
				{Label: "Dashboard", URL: "/"},
				{Label: projectName},
			}},
			ProjectName: projectName,
			Groups:      groups,
		})
	}
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
			basePageData: basePageData{Breadcrumbs: []breadcrumb{
				{Label: "Dashboard", URL: "/"},
				{Label: projectName, URL: "/project/" + projectName},
				{Label: "New Feature"},
			}},
			ProjectName: projectName,
		})
	}
}

// handleWebCreateFeature handles POST /project/{name}/features from the web form.
func handleWebCreateFeature(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/project/"+projectName, http.StatusFound)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		description := r.FormValue("description")
		if name == "" {
			http.Redirect(w, r, "/project/"+projectName+"/new", http.StatusFound)
			return
		}
		if _, err := st.CreateFeature(projectName, name, description); err != nil {
			http.Redirect(w, r, "/project/"+projectName, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/project/"+projectName, http.StatusFound)
	}
}

// --- Feature detail handler ---

// featureIterationPageData holds data for one dialog iteration on the feature page.
type featureIterationPageData struct {
	Round       int
	Description string
	Questions   string
	Response    string
	IsFinal     bool
	IsLast      bool
}

type featureDetailPageData struct {
	basePageData
	ProjectName        string
	Feature            featureRowData
	InitialDescription string
	CurrentDescription string
	LatestQuestions    string
	Iterations         []featureIterationPageData
	OtherFeatures      []featureRowData
}

func handleWebFeature(st Store) http.HandlerFunc {
	tmpl := mustParseTemplate("feature")
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		detail, err := st.GetFeatureDetail(projectName, featureID)
		if err != nil {
			http.NotFound(w, r)
			return
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
		// Searching backwards handles the case where a reopen creates a new iteration
		// with no description yet (only a response file).
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
		// Only needed when the feature is fully_specified (the only state showing the dropdown).
		var otherFeatures []featureRowData
		if detail.Status == model.StatusFullySpecified {
			if allFeatures, err := st.ListFeatures(projectName, nil); err == nil {
				for _, f := range allFeatures {
					if f.ID != featureID &&
						f.Status != model.StatusDone &&
						f.Status != model.StatusAbandoned &&
						f.Status != model.StatusHalted {
						otherFeatures = append(otherFeatures, featureRowData{
							ID:     f.ID,
							Name:   f.Name,
							Status: f.Status.String(),
						})
					}
				}
			}
		}

		tmpl.Execute(w, featureDetailPageData{
			basePageData: basePageData{Breadcrumbs: []breadcrumb{
				{Label: "Dashboard", URL: "/"},
				{Label: projectName, URL: "/project/" + projectName},
				{Label: detail.Name},
			}},
			ProjectName: projectName,
			Feature: featureRowData{
				ID:        detail.ID,
				Name:      detail.Name,
				Status:    detail.Status.String(),
				UpdatedAt: detail.UpdatedAt.Format("2006-01-02 15:04"),
			},
			InitialDescription: detail.InitialDescription,
			CurrentDescription: currentDesc,
			LatestQuestions:    latestQuestions,
			Iterations:         iterations,
			OtherFeatures:      otherFeatures,
		})
	}
}

// handleWebUpdateDescription handles POST /project/{name}/feature/{id}/description.
// It updates description_v0.md for a draft feature and redirects back.
func handleWebUpdateDescription(st Store) http.HandlerFunc {
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
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}

// handleWebStartDialog handles POST /project/{name}/feature/{id}/start-dialog.
// It starts the dialog for a draft feature and redirects back.
func handleWebStartDialog(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		_ = st.StartDialog(projectName, featureID)
		http.Redirect(w, r, "/project/"+projectName+"/feature/"+featureID, http.StatusFound)
	}
}

// handleWebRespond handles POST /project/{name}/feature/{id}/respond.
// It stores the user's response and redirects back. The "final" form field controls
// whether this is marked as a final response.
func handleWebRespond(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		featurePage := "/project/" + projectName + "/feature/" + featureID
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, featurePage, http.StatusFound)
			return
		}
		response := r.FormValue("response")
		final := r.FormValue("final") == "true"
		_ = st.RespondToDialog(projectName, featureID, response, final)
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}

// handleWebReopen handles POST /project/{name}/feature/{id}/reopen.
// It reopens a fully-specified feature dialog and redirects back.
func handleWebReopen(st Store) http.HandlerFunc {
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
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}

// handleWebGenerateNow handles POST /project/{name}/feature/{id}/generate-now.
// Transitions a fully_specified feature to ready_to_generate and redirects back.
func handleWebGenerateNow(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		_ = st.TransitionStatus(projectName, featureID, model.StatusReadyToGenerate)
		http.Redirect(w, r, "/project/"+projectName+"/feature/"+featureID, http.StatusFound)
	}
}

// handleWebGenerateAfter handles POST /project/{name}/feature/{id}/generate-after.
// Sets a dependency on another feature and transitions to waiting, then redirects back.
func handleWebGenerateAfter(st Store) http.HandlerFunc {
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
		http.Redirect(w, r, featurePage, http.StatusFound)
	}
}
