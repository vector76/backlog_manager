// Package store provides file-based persistence for projects and features.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vector76/backlog_manager/internal/model"
)

// validTransitions maps each source status to the set of valid target statuses.
// All statuses may transition to Abandoned or Halted (handled separately).
var validTransitions = map[model.FeatureStatus]map[model.FeatureStatus]bool{
	model.StatusDraft: {
		model.StatusAwaitingClient: true,
	},
	model.StatusAwaitingClient: {
		model.StatusAwaitingHuman:  true,
		model.StatusFullySpecified: true,
	},
	model.StatusAwaitingHuman: {
		model.StatusAwaitingClient: true,
	},
	model.StatusFullySpecified: {
		model.StatusAwaitingClient:  true,
		model.StatusReadyToGenerate: true,
		model.StatusWaiting:         true,
	},
	model.StatusWaiting: {
		model.StatusReadyToGenerate: true,
	},
	model.StatusReadyToGenerate: {
		model.StatusGenerating: true,
	},
	model.StatusGenerating: {
		model.StatusBeadsCreated: true,
	},
	model.StatusBeadsCreated: {
		model.StatusDone: true,
	},
	model.StatusDone:      {},
	model.StatusAbandoned: {},
	model.StatusHalted:    {},
}

// IterationContent holds the content of a single dialog iteration (round >= 1).
type IterationContent struct {
	Round       int
	Description string
	Questions   string // empty if client submitted no questions
	Response    string // empty until human responds
}

// FeatureDetail includes a Feature plus its full dialog history.
type FeatureDetail struct {
	model.Feature
	InitialDescription string
	Iterations         []IterationContent
}

// projectsRegistry is the JSON structure for projects.json.
type projectsRegistry struct {
	Projects []model.Project `json:"projects"`
}

// featuresRegistry is the JSON structure for a project's features.json.
type featuresRegistry struct {
	Features []model.Feature `json:"features"`
}

// Store is the file-based storage backend.
type Store struct {
	mu       sync.RWMutex
	dataDir  string
	projects []model.Project
	features map[string][]model.Feature // project name -> features
}

// New creates or opens a Store rooted at dataDir, loading existing data from disk.
func New(dataDir string) (*Store, error) {
	s := &Store{
		dataDir:  dataDir,
		features: make(map[string][]model.Feature),
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := s.loadProjects(); err != nil {
		return nil, err
	}
	for _, p := range s.projects {
		if err := s.loadFeatures(p.Name); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// --- Path helpers ---

func (s *Store) projectsFile() string {
	return filepath.Join(s.dataDir, "projects.json")
}

func (s *Store) projectDir(projectName string) string {
	return filepath.Join(s.dataDir, projectName)
}

func (s *Store) featuresFile(projectName string) string {
	return filepath.Join(s.projectDir(projectName), "features.json")
}

func (s *Store) featureDir(projectName, featureID string) string {
	return filepath.Join(s.projectDir(projectName), "features", featureID)
}

func (s *Store) descriptionPath(projectName, featureID string, version int) string {
	return filepath.Join(s.featureDir(projectName, featureID), fmt.Sprintf("description_v%d.md", version))
}

func (s *Store) questionsPath(projectName, featureID string, round int) string {
	return filepath.Join(s.featureDir(projectName, featureID), fmt.Sprintf("questions_v%d.md", round))
}

func (s *Store) responsePath(projectName, featureID string, round int) string {
	return filepath.Join(s.featureDir(projectName, featureID), fmt.Sprintf("response_v%d.md", round))
}

func (s *Store) artifactPath(projectName, featureID, name string) string {
	return filepath.Join(s.featureDir(projectName, featureID), name)
}

// --- Atomic write ---

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		os.Remove(tmp) // best-effort cleanup of partial file
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// --- Load/save ---

func (s *Store) loadProjects() error {
	data, err := os.ReadFile(s.projectsFile())
	if errors.Is(err, os.ErrNotExist) {
		s.projects = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("read projects.json: %w", err)
	}
	var reg projectsRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return fmt.Errorf("parse projects.json: %w", err)
	}
	s.projects = reg.Projects
	return nil
}

func (s *Store) saveProjects() error {
	reg := projectsRegistry{Projects: s.projects}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal projects: %w", err)
	}
	return atomicWrite(s.projectsFile(), data)
}

func (s *Store) loadFeatures(projectName string) error {
	data, err := os.ReadFile(s.featuresFile(projectName))
	if errors.Is(err, os.ErrNotExist) {
		s.features[projectName] = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("read features.json for %s: %w", projectName, err)
	}
	var reg featuresRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return fmt.Errorf("parse features.json for %s: %w", projectName, err)
	}
	s.features[projectName] = reg.Features
	return nil
}

func (s *Store) saveFeatures(projectName string) error {
	reg := featuresRegistry{Features: s.features[projectName]}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal features for %s: %w", projectName, err)
	}
	return atomicWrite(s.featuresFile(projectName), data)
}

// --- Project CRUD ---

// CreateProject creates a new project with the given name and token.
func (s *Store) CreateProject(name, token string) (*model.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.projects {
		if p.Name == name {
			return nil, fmt.Errorf("project %q already exists", name)
		}
	}

	p := model.Project{
		Name:      name,
		Token:     token,
		CreatedAt: time.Now().UTC(),
	}
	s.projects = append(s.projects, p)
	if err := s.saveProjects(); err != nil {
		s.projects = s.projects[:len(s.projects)-1]
		return nil, err
	}

	// Initialize empty feature list
	s.features[name] = nil
	if err := s.saveFeatures(name); err != nil {
		// Roll back the project
		s.projects = s.projects[:len(s.projects)-1]
		delete(s.features, name)
		_ = s.saveProjects()
		return nil, err
	}

	return &p, nil
}

// GetProjectByToken returns the project with the given token, or an error if not found.
func (s *Store) GetProjectByToken(token string) (*model.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.Token == token {
			cp := p
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("no project found for token")
}

// ListProjects returns all projects.
func (s *Store) ListProjects() []model.Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.Project, len(s.projects))
	copy(result, s.projects)
	return result
}

// GetProject returns the project with the given name, or an error if not found.
func (s *Store) GetProject(name string) (*model.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.Name == name {
			cp := p
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("project %q not found", name)
}

// DeleteProject removes a project and all its data.
func (s *Store) DeleteProject(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, p := range s.projects {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("project %q not found", name)
	}

	// Remove directory
	if err := os.RemoveAll(s.projectDir(name)); err != nil {
		return fmt.Errorf("remove project dir: %w", err)
	}

	// Remove from in-memory state (take copies so we can roll back if save fails).
	origProjects := make([]model.Project, len(s.projects))
	copy(origProjects, s.projects)
	origFeatures := s.features[name]
	s.projects = append(s.projects[:idx], s.projects[idx+1:]...)
	delete(s.features, name)

	if err := s.saveProjects(); err != nil {
		s.projects = origProjects
		s.features[name] = origFeatures
		return err
	}
	return nil
}

// --- Feature CRUD ---

// CreateFeature creates a new feature in a project with an initial description.
func (s *Store) CreateFeature(projectName, featureName, description string) (*model.Feature, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.getProjectLocked(projectName); err != nil {
		return nil, err
	}

	id, err := model.GenerateFeatureID(func(candidate string) bool {
		for _, f := range s.features[projectName] {
			if f.ID == candidate {
				return true
			}
		}
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("generate feature id: %w", err)
	}

	now := time.Now().UTC()
	f := model.Feature{
		ID:               id,
		Project:          projectName,
		Name:             featureName,
		Status:           model.StatusDraft,
		CurrentIteration: 0,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// Create feature directory
	fdir := s.featureDir(projectName, id)
	if err := os.MkdirAll(fdir, 0755); err != nil {
		return nil, fmt.Errorf("create feature dir: %w", err)
	}

	// Write description_v0.md
	if err := os.WriteFile(s.descriptionPath(projectName, id, 0), []byte(description), 0644); err != nil {
		os.RemoveAll(fdir)
		return nil, fmt.Errorf("write description_v0.md: %w", err)
	}

	s.features[projectName] = append(s.features[projectName], f)
	if err := s.saveFeatures(projectName); err != nil {
		s.features[projectName] = s.features[projectName][:len(s.features[projectName])-1]
		os.RemoveAll(fdir)
		return nil, err
	}

	return &f, nil
}

// ListFeatures returns features for a project, optionally filtered by status.
func (s *Store) ListFeatures(projectName string, statusFilter *model.FeatureStatus) ([]model.Feature, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := s.getProjectLocked(projectName); err != nil {
		return nil, err
	}

	var result []model.Feature
	for _, f := range s.features[projectName] {
		if statusFilter == nil || f.Status == *statusFilter {
			cp := f
			result = append(result, cp)
		}
	}
	return result, nil
}

// GetFeature returns a feature by project and feature ID.
func (s *Store) GetFeature(projectName, featureID string) (*model.Feature, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getFeatureLocked(projectName, featureID)
}

// GetFeatureDetail returns a feature along with its full dialog history read from disk.
func (s *Store) GetFeatureDetail(projectName, featureID string) (*FeatureDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := s.getFeatureLocked(projectName, featureID)
	if err != nil {
		return nil, err
	}
	iterations := f.CurrentIteration

	detail := &FeatureDetail{Feature: *f}

	// Read initial description (v0)
	v0, err := readFileOptional(s.descriptionPath(projectName, featureID, 0))
	if err != nil {
		return nil, err
	}
	detail.InitialDescription = v0

	// Read dialog rounds 1..N
	for round := 1; round <= iterations; round++ {
		ic := IterationContent{Round: round}
		ic.Description, err = readFileOptional(s.descriptionPath(projectName, featureID, round))
		if err != nil {
			return nil, err
		}
		ic.Questions, err = readFileOptional(s.questionsPath(projectName, featureID, round))
		if err != nil {
			return nil, err
		}
		ic.Response, err = readFileOptional(s.responsePath(projectName, featureID, round))
		if err != nil {
			return nil, err
		}
		detail.Iterations = append(detail.Iterations, ic)
	}

	return detail, nil
}

// UpdateFeature updates the metadata for a feature.
func (s *Store) UpdateFeature(updated *model.Feature) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	features, ok := s.features[updated.Project]
	if !ok {
		return fmt.Errorf("project %q not found", updated.Project)
	}
	for i, f := range features {
		if f.ID == updated.ID {
			old := s.features[updated.Project][i]
			cp := *updated
			cp.UpdatedAt = time.Now().UTC()
			s.features[updated.Project][i] = cp
			if err := s.saveFeatures(updated.Project); err != nil {
				s.features[updated.Project][i] = old
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("feature %q not found in project %q", updated.ID, updated.Project)
}

// DeleteFeature removes a feature and its files.
func (s *Store) DeleteFeature(projectName, featureID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	features, ok := s.features[projectName]
	if !ok {
		return fmt.Errorf("project %q not found", projectName)
	}
	idx := -1
	for i, f := range features {
		if f.ID == featureID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("feature %q not found in project %q", featureID, projectName)
	}

	// Remove files
	if err := os.RemoveAll(s.featureDir(projectName, featureID)); err != nil {
		return fmt.Errorf("remove feature dir: %w", err)
	}

	// Take a copy before mutating the backing array, so we can roll back if save fails.
	orig := make([]model.Feature, len(features))
	copy(orig, features)
	s.features[projectName] = append(features[:idx], features[idx+1:]...)
	if err := s.saveFeatures(projectName); err != nil {
		s.features[projectName] = orig
		return err
	}
	return nil
}

// --- Dialog iteration management ---

// WriteClientRound writes description_vN.md and questions_vN.md for a dialog round.
// round must be >= 1. This also increments CurrentIteration on the feature.
func (s *Store) WriteClientRound(projectName, featureID string, round int, description, questions string) error {
	if round < 1 {
		return fmt.Errorf("round must be >= 1, got %d", round)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.getFeatureLocked(projectName, featureID)
	if err != nil {
		return err
	}

	if err := os.WriteFile(s.descriptionPath(projectName, featureID, round), []byte(description), 0644); err != nil {
		return fmt.Errorf("write description_v%d.md: %w", round, err)
	}
	if err := os.WriteFile(s.questionsPath(projectName, featureID, round), []byte(questions), 0644); err != nil {
		return fmt.Errorf("write questions_v%d.md: %w", round, err)
	}

	// Update CurrentIteration if this is a new round
	if round > f.CurrentIteration {
		for i, feat := range s.features[projectName] {
			if feat.ID == featureID {
				oldIteration := s.features[projectName][i].CurrentIteration
				oldUpdatedAt := s.features[projectName][i].UpdatedAt
				s.features[projectName][i].CurrentIteration = round
				s.features[projectName][i].UpdatedAt = time.Now().UTC()
				if err := s.saveFeatures(projectName); err != nil {
					s.features[projectName][i].CurrentIteration = oldIteration
					s.features[projectName][i].UpdatedAt = oldUpdatedAt
					return err
				}
				break
			}
		}
	}
	return nil
}

// WriteHumanResponse writes response_vN.md for a dialog round.
func (s *Store) WriteHumanResponse(projectName, featureID string, round int, response string) error {
	if round < 1 {
		return fmt.Errorf("round must be >= 1, got %d", round)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.getFeatureLocked(projectName, featureID); err != nil {
		return err
	}

	return os.WriteFile(s.responsePath(projectName, featureID, round), []byte(response), 0644)
}

// ReadDescriptionVersion reads description_vN.md (N=0 is the initial description).
func (s *Store) ReadDescriptionVersion(projectName, featureID string, version int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getFeatureLocked(projectName, featureID); err != nil {
		return "", err
	}
	return readFileOptional(s.descriptionPath(projectName, featureID, version))
}

// ReadQuestions reads questions_vN.md for a dialog round.
func (s *Store) ReadQuestions(projectName, featureID string, round int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getFeatureLocked(projectName, featureID); err != nil {
		return "", err
	}
	return readFileOptional(s.questionsPath(projectName, featureID, round))
}

// ReadResponse reads response_vN.md for a dialog round.
func (s *Store) ReadResponse(projectName, featureID string, round int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getFeatureLocked(projectName, featureID); err != nil {
		return "", err
	}
	return readFileOptional(s.responsePath(projectName, featureID, round))
}

// WriteArtifact writes a named artifact file (e.g., "plan.md", "beads.md") for a feature.
func (s *Store) WriteArtifact(projectName, featureID, name, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getFeatureLocked(projectName, featureID); err != nil {
		return err
	}
	return os.WriteFile(s.artifactPath(projectName, featureID, name), []byte(content), 0644)
}

// ReadArtifact reads a named artifact file for a feature.
func (s *Store) ReadArtifact(projectName, featureID, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getFeatureLocked(projectName, featureID); err != nil {
		return "", err
	}
	return readFileOptional(s.artifactPath(projectName, featureID, name))
}

// --- Status transitions ---

// TransitionStatus changes a feature's status, validating the transition.
func (s *Store) TransitionStatus(projectName, featureID string, newStatus model.FeatureStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	features, ok := s.features[projectName]
	if !ok {
		return fmt.Errorf("project %q not found", projectName)
	}
	idx := -1
	for i, f := range features {
		if f.ID == featureID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("feature %q not found in project %q", featureID, projectName)
	}

	current := s.features[projectName][idx].Status
	if err := validateTransition(current, newStatus); err != nil {
		return err
	}

	oldUpdatedAt := s.features[projectName][idx].UpdatedAt
	s.features[projectName][idx].Status = newStatus
	s.features[projectName][idx].UpdatedAt = time.Now().UTC()
	if err := s.saveFeatures(projectName); err != nil {
		s.features[projectName][idx].Status = current
		s.features[projectName][idx].UpdatedAt = oldUpdatedAt
		return err
	}
	return nil
}

// ValidateTransition checks if a status transition is allowed.
func ValidateTransition(from, to model.FeatureStatus) error {
	return validateTransition(from, to)
}

func validateTransition(from, to model.FeatureStatus) error {
	// Any status can transition to abandoned or halted
	if to == model.StatusAbandoned || to == model.StatusHalted {
		return nil
	}
	targets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("invalid source status %v", from)
	}
	if !targets[to] {
		return fmt.Errorf("invalid status transition from %v to %v", from, to)
	}
	return nil
}

// --- Locked helpers (must be called with mu held) ---

func (s *Store) getProjectLocked(name string) (*model.Project, error) {
	for _, p := range s.projects {
		if p.Name == name {
			cp := p
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("project %q not found", name)
}

func (s *Store) getFeatureLocked(projectName, featureID string) (*model.Feature, error) {
	features, ok := s.features[projectName]
	if !ok {
		return nil, fmt.Errorf("project %q not found", projectName)
	}
	for _, f := range features {
		if f.ID == featureID {
			cp := f
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("feature %q not found in project %q", featureID, projectName)
}

// --- File helpers ---

// readFileOptional reads a file, returning "" if it doesn't exist.
func readFileOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}
