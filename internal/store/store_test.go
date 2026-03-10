package store

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vector76/backlog_manager/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "bm-store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// --- Project CRUD ---

func TestCreateProject(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject("proj1", "token1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "proj1" || p.Token != "token1" {
		t.Errorf("unexpected project: %+v", p)
	}
}

func TestCreateProjectDuplicate(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("proj1", "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject("proj1", "tok2"); err == nil {
		t.Error("expected error for duplicate project name")
	}
}

func TestListProjects(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("a", "ta"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject("b", "tb"); err != nil {
		t.Fatal(err)
	}
	projects := s.ListProjects()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestGetProject(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("myproj", "tok"); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetProject("myproj")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "myproj" {
		t.Errorf("got name %q, want %q", p.Name, "myproj")
	}
}

func TestGetProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetProject("nope"); err == nil {
		t.Error("expected error for missing project")
	}
}

func TestDeleteProject(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("del", "tok"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject("del"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject("del"); err == nil {
		t.Error("expected project to be gone after delete")
	}
	if len(s.ListProjects()) != 0 {
		t.Error("expected 0 projects after delete")
	}
}

func TestDeleteProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteProject("nope"); err == nil {
		t.Error("expected error deleting missing project")
	}
}

// --- Feature CRUD ---

func TestCreateFeature(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "My Feature", "Initial description", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "My Feature" {
		t.Errorf("got name %q", f.Name)
	}
	if f.Status != model.StatusDraft {
		t.Errorf("expected draft status, got %v", f.Status)
	}
	if f.CurrentIteration != 0 {
		t.Errorf("expected CurrentIteration=0, got %d", f.CurrentIteration)
	}
}

func TestCreateFeatureWritesDescriptionV0(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "hello world", false, "")
	if err != nil {
		t.Fatal(err)
	}
	content, err := s.ReadDescriptionVersion("p", f.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello world" {
		t.Errorf("description_v0.md: got %q, want %q", content, "hello world")
	}
}

func TestCreateFeatureUnknownProject(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateFeature("nope", "f", "desc", false, ""); err == nil {
		t.Error("expected error for unknown project")
	}
}

func TestCreateFeatureDirectToBeadReadyToGenerate(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusReadyToGenerate {
		t.Errorf("expected ready_to_generate, got %v", f.Status)
	}
	if !f.DirectToBead {
		t.Error("expected DirectToBead=true")
	}
	if f.GenerateAfter != "" {
		t.Errorf("expected empty GenerateAfter, got %q", f.GenerateAfter)
	}
}

func TestCreateFeatureDirectToBeadWithDependency(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", true, "dep-id")
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusWaiting {
		t.Errorf("expected waiting, got %v", f.Status)
	}
	if !f.DirectToBead {
		t.Error("expected DirectToBead=true")
	}
	if f.GenerateAfter != "dep-id" {
		t.Errorf("expected GenerateAfter=%q, got %q", "dep-id", f.GenerateAfter)
	}
}

func TestCreateFeatureDirectToBeadFalseIgnoresGenerateAfter(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "some-id")
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusDraft {
		t.Errorf("expected draft, got %v", f.Status)
	}
	if f.DirectToBead {
		t.Error("expected DirectToBead=false")
	}
	if f.GenerateAfter != "" {
		t.Errorf("expected empty GenerateAfter, got %q", f.GenerateAfter)
	}
}

func TestCreateFeature_DirectToBead_WritesDescriptionV0(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "hello direct to bead", true, "")
	if err != nil {
		t.Fatal(err)
	}
	content, err := s.ReadDescriptionVersion("p", f.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello direct to bead" {
		t.Errorf("description_v0.md: got %q, want %q", content, "hello direct to bead")
	}
}

func TestListFeatures(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFeature("p", "f1", "d1", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFeature("p", "f2", "d2", false, ""); err != nil {
		t.Fatal(err)
	}
	features, err := s.ListFeatures("p", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(features) != 2 {
		t.Errorf("expected 2 features, got %d", len(features))
	}
}

func TestListFeaturesWithStatusFilter(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f1, err := s.CreateFeature("p", "f1", "d1", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFeature("p", "f2", "d2", false, ""); err != nil {
		t.Fatal(err)
	}
	// Transition f1 to awaiting_client
	if err := s.TransitionStatus("p", f1.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}

	status := model.StatusDraft
	features, err := s.ListFeatures("p", &status)
	if err != nil {
		t.Fatal(err)
	}
	if len(features) != 1 {
		t.Errorf("expected 1 draft feature, got %d", len(features))
	}
}

func TestGetFeature(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFeature("p", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID || got.Name != created.Name {
		t.Errorf("got %+v, want %+v", got, created)
	}
}

func TestGetFeatureNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFeature("p", "ft-nope"); err == nil {
		t.Error("expected error for missing feature")
	}
}

func TestUpdateFeature(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	originalUpdatedAt := f.UpdatedAt
	f.Name = "Updated Name"
	if err := s.UpdateFeature(f); err != nil {
		t.Fatal(err)
	}
	// UpdateFeature must not mutate the caller's struct
	if f.UpdatedAt != originalUpdatedAt {
		t.Error("UpdateFeature must not modify the caller's UpdatedAt field")
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("got name %q, want %q", got.Name, "Updated Name")
	}
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Error("expected stored UpdatedAt to be after original")
	}
}

func TestDeleteFeature(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFeature("p", f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFeature("p", f.ID); err == nil {
		t.Error("expected error after feature deletion")
	}
}

func TestDeleteFeatureNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFeature("p", "ft-nope"); err == nil {
		t.Error("expected error deleting missing feature")
	}
}

func TestUpdateDescriptionV0_InDraft(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "original", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDescriptionV0("p", f.ID, "updated"); err != nil {
		t.Fatal(err)
	}
	content, err := s.ReadDescriptionVersion("p", f.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if content != "updated" {
		t.Errorf("expected 'updated', got %q", content)
	}
}

func TestUpdateDescriptionV0_NotDraft(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "original", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionStatus("p", f.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDescriptionV0("p", f.ID, "updated"); err == nil {
		t.Error("expected error updating description of non-draft feature")
	}
	// Original content should be unchanged
	content, err := s.ReadDescriptionVersion("p", f.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if content != "original" {
		t.Errorf("expected 'original' unchanged, got %q", content)
	}
}

// --- Dialog iteration management ---

func TestWriteAndReadClientRound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "initial", false, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.WriteClientRound("p", f.ID, 1, "desc v1", "questions v1"); err != nil {
		t.Fatal(err)
	}

	desc, err := s.ReadDescriptionVersion("p", f.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if desc != "desc v1" {
		t.Errorf("got %q, want %q", desc, "desc v1")
	}

	questions, err := s.ReadQuestions("p", f.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if questions != "questions v1" {
		t.Errorf("got %q, want %q", questions, "questions v1")
	}
}

func TestWriteAndReadHumanResponse(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "initial", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteClientRound("p", f.ID, 1, "d1", "q1"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteHumanResponse("p", f.ID, 1, "user response"); err != nil {
		t.Fatal(err)
	}

	resp, err := s.ReadResponse("p", f.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if resp != "user response" {
		t.Errorf("got %q, want %q", resp, "user response")
	}
}

func TestWriteClientRoundInvalidRound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteClientRound("p", f.ID, 0, "d", "q"); err == nil {
		t.Error("expected error for round=0")
	}
}

func TestWriteClientRoundUpdatesCurrentIteration(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteClientRound("p", f.ID, 1, "d1", "q1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentIteration != 1 {
		t.Errorf("expected CurrentIteration=1, got %d", got.CurrentIteration)
	}
}

func TestGetFeatureDetail(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "initial desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteClientRound("p", f.ID, 1, "desc v1", "questions v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteHumanResponse("p", f.ID, 1, "response v1"); err != nil {
		t.Fatal(err)
	}

	detail, err := s.GetFeatureDetail("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.InitialDescription != "initial desc" {
		t.Errorf("InitialDescription: got %q", detail.InitialDescription)
	}
	if len(detail.Iterations) != 1 {
		t.Fatalf("expected 1 iteration, got %d", len(detail.Iterations))
	}
	it := detail.Iterations[0]
	if it.Round != 1 {
		t.Errorf("Round: got %d", it.Round)
	}
	if it.Description != "desc v1" {
		t.Errorf("Description: got %q", it.Description)
	}
	if it.Questions != "questions v1" {
		t.Errorf("Questions: got %q", it.Questions)
	}
	if it.Response != "response v1" {
		t.Errorf("Response: got %q", it.Response)
	}
}

func TestGetFeatureDetailMultipleRounds(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "initial", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Round 1
	if err := s.WriteClientRound("p", f.ID, 1, "desc v1", "q1"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteHumanResponse("p", f.ID, 1, "resp1"); err != nil {
		t.Fatal(err)
	}

	// Round 2
	if err := s.WriteClientRound("p", f.ID, 2, "desc v2", "q2"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteHumanResponse("p", f.ID, 2, "resp2"); err != nil {
		t.Fatal(err)
	}

	detail, err := s.GetFeatureDetail("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Iterations) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(detail.Iterations))
	}
	if detail.Iterations[0].Round != 1 || detail.Iterations[0].Description != "desc v1" ||
		detail.Iterations[0].Questions != "q1" || detail.Iterations[0].Response != "resp1" {
		t.Errorf("round 1 mismatch: %+v", detail.Iterations[0])
	}
	if detail.Iterations[1].Round != 2 || detail.Iterations[1].Description != "desc v2" ||
		detail.Iterations[1].Questions != "q2" || detail.Iterations[1].Response != "resp2" {
		t.Errorf("round 2 mismatch: %+v", detail.Iterations[1])
	}
	// CurrentIteration should reflect the highest round written
	if detail.Feature.CurrentIteration != 2 {
		t.Errorf("expected CurrentIteration=2, got %d", detail.Feature.CurrentIteration)
	}
}

func TestListFeaturesUnknownProject(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ListFeatures("nope", nil); err == nil {
		t.Error("expected error listing features for unknown project")
	}
}

// Operations on a valid feature ID but wrong/nonexistent project should say
// "project not found", not "feature not found".
func TestUnknownProjectErrors(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	assertProjectNotFound := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s: expected error for unknown project, got nil", name)
			return
		}
		if got := err.Error(); got != `project "nope" not found` {
			t.Errorf("%s: expected project-not-found error, got: %v", name, err)
		}
	}

	_, err = s.GetFeature("nope", f.ID)
	assertProjectNotFound("GetFeature", err)

	_, err = s.GetFeatureDetail("nope", f.ID)
	assertProjectNotFound("GetFeatureDetail", err)

	assertProjectNotFound("TransitionStatus",
		s.TransitionStatus("nope", f.ID, model.StatusAwaitingClient))

	assertProjectNotFound("DeleteFeature",
		s.DeleteFeature("nope", f.ID))

	assertProjectNotFound("WriteClientRound",
		s.WriteClientRound("nope", f.ID, 1, "d", "q"))

	assertProjectNotFound("UpdateFeature",
		s.UpdateFeature(&model.Feature{ID: f.ID, Project: "nope", Name: "x", Status: model.StatusDraft}))
}

func TestReadMissingFileReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	content, err := s.ReadDescriptionVersion("p", f.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Errorf("expected empty string for missing file, got %q", content)
	}
}

// --- Artifact ---

func TestWriteAndReadArtifact(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteArtifact("p", f.ID, "plan.md", "the plan"); err != nil {
		t.Fatal(err)
	}
	content, err := s.ReadArtifact("p", f.ID, "plan.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "the plan" {
		t.Errorf("got %q, want %q", content, "the plan")
	}
}

// --- Status transitions ---

func TestValidTransitions(t *testing.T) {
	cases := []struct {
		from model.FeatureStatus
		to   model.FeatureStatus
	}{
		{model.StatusDraft, model.StatusAwaitingClient},
		{model.StatusAwaitingClient, model.StatusAwaitingHuman},
		{model.StatusAwaitingClient, model.StatusFullySpecified},
		{model.StatusAwaitingHuman, model.StatusAwaitingClient},
		{model.StatusFullySpecified, model.StatusAwaitingClient},
		{model.StatusFullySpecified, model.StatusReadyToGenerate},
		{model.StatusFullySpecified, model.StatusWaiting},
		{model.StatusWaiting, model.StatusReadyToGenerate},
		{model.StatusReadyToGenerate, model.StatusGenerating},
		{model.StatusGenerating, model.StatusBeadsCreated},
		{model.StatusBeadsCreated, model.StatusDone},
		// Any -> abandoned/halted
		{model.StatusDraft, model.StatusAbandoned},
		{model.StatusDraft, model.StatusHalted},
		{model.StatusGenerating, model.StatusAbandoned},
		{model.StatusDone, model.StatusAbandoned},
	}
	for _, c := range cases {
		if err := ValidateTransition(c.from, c.to); err != nil {
			t.Errorf("expected valid transition %v->%v, got error: %v", c.from, c.to, err)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	cases := []struct {
		from model.FeatureStatus
		to   model.FeatureStatus
	}{
		{model.StatusDraft, model.StatusFullySpecified},
		{model.StatusDraft, model.StatusGenerating},
		{model.StatusAwaitingClient, model.StatusDraft},
		{model.StatusDone, model.StatusDraft},
		{model.StatusDone, model.StatusGenerating},
		{model.StatusAbandoned, model.StatusDraft},
		{model.StatusBeadsCreated, model.StatusAwaitingClient},
	}
	for _, c := range cases {
		if err := ValidateTransition(c.from, c.to); err == nil {
			t.Errorf("expected invalid transition %v->%v to fail, but got nil", c.from, c.to)
		}
	}
}

func TestTransitionStatus(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.TransitionStatus("p", f.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusAwaitingClient {
		t.Errorf("expected awaiting_client, got %v", got.Status)
	}
}

func TestWriteArtifact(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteArtifact("p", f.ID, "plan.md", "# Plan\nDo things."); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	got, err := s.ReadArtifact("p", f.ID, "plan.md")
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if got != "# Plan\nDo things." {
		t.Errorf("got %q, want %q", got, "# Plan\nDo things.")
	}
}

func TestWriteArtifact_BeadsType(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteArtifact("p", f.ID, "beads.md", "# Beads\nbd-xxxx"); err != nil {
		t.Fatalf("WriteArtifact: %v", err)
	}
	got, err := s.ReadArtifact("p", f.ID, "beads.md")
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if got != "# Beads\nbd-xxxx" {
		t.Errorf("got %q, want %q", got, "# Beads\nbd-xxxx")
	}
}

func TestTransitionStatus_GenerationPipeline(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	pipeline := []model.FeatureStatus{
		model.StatusAwaitingClient,
		model.StatusFullySpecified,
		model.StatusReadyToGenerate,
		model.StatusGenerating,
		model.StatusBeadsCreated,
		model.StatusDone,
	}
	for _, s2 := range pipeline {
		if err := s.TransitionStatus("p", f.ID, s2); err != nil {
			t.Fatalf("transition to %v: %v", s2, err)
		}
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusDone {
		t.Errorf("expected done, got %v", got.Status)
	}
}

func TestTransitionStatus_WaitingPipeline(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	// FullySpecified -> Waiting -> ReadyToGenerate
	for _, st := range []model.FeatureStatus{model.StatusAwaitingClient, model.StatusFullySpecified, model.StatusWaiting, model.StatusReadyToGenerate} {
		if err := s.TransitionStatus("p", f.ID, st); err != nil {
			t.Fatalf("transition to %v: %v", st, err)
		}
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusReadyToGenerate {
		t.Errorf("expected ready_to_generate, got %v", got.Status)
	}
}

func TestTransitionStatusInvalid(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	// draft -> fully_specified is invalid
	if err := s.TransitionStatus("p", f.ID, model.StatusFullySpecified); err == nil {
		t.Error("expected error for invalid transition")
	}
	// Status should remain draft
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusDraft {
		t.Errorf("status should still be draft, got %v", got.Status)
	}
}

func TestAbandonFromAnyStatus(t *testing.T) {
	statuses := []model.FeatureStatus{
		model.StatusDraft, model.StatusAwaitingClient, model.StatusAwaitingHuman,
		model.StatusFullySpecified, model.StatusWaiting, model.StatusReadyToGenerate,
		model.StatusGenerating, model.StatusBeadsCreated, model.StatusDone,
	}
	for _, status := range statuses {
		s := newTestStore(t)
		if _, err := s.CreateProject("p", "tok"); err != nil {
			t.Fatal(err)
		}
		f, err := s.CreateFeature("p", "feat", "desc", false, "")
		if err != nil {
			t.Fatal(err)
		}
		// Force the status in-memory for testing
		s.mu.Lock()
		for i, feat := range s.features["p"] {
			if feat.ID == f.ID {
				s.features["p"][i].Status = status
			}
		}
		s.mu.Unlock()

		if err := s.TransitionStatus("p", f.ID, model.StatusAbandoned); err != nil {
			t.Errorf("status %v -> abandoned should be valid, got error: %v", status, err)
		}
	}
}

// --- Persistence across restarts ---

func TestPersistenceProjects(t *testing.T) {
	dir, err := os.MkdirTemp("", "bm-persist-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s1, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.CreateProject("persist-proj", "tok"); err != nil {
		t.Fatal(err)
	}

	// Reload from same directory
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s2.GetProject("persist-proj")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "persist-proj" {
		t.Errorf("got %q after reload", p.Name)
	}
}

func TestPersistenceFeatures(t *testing.T) {
	dir, err := os.MkdirTemp("", "bm-persist-feat-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s1, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.CreateProject("proj", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s1.CreateFeature("proj", "my-feat", "initial desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.TransitionStatus("proj", f.ID, model.StatusAwaitingClient); err != nil {
		t.Fatal(err)
	}

	// Reload
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.GetFeature("proj", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "my-feat" {
		t.Errorf("name: got %q, want %q", got.Name, "my-feat")
	}
	if got.Status != model.StatusAwaitingClient {
		t.Errorf("status: got %v, want awaiting_client", got.Status)
	}

	// The description_v0.md should also be there
	content, err := s2.ReadDescriptionVersion("proj", f.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if content != "initial desc" {
		t.Errorf("description_v0: got %q", content)
	}
}

func TestPersistenceDirectToBead(t *testing.T) {
	dir, err := os.MkdirTemp("", "bm-persist-dtb-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s1, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.CreateProject("proj", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s1.CreateFeature("proj", "dtb-feat", "desc", true, "dep-id")
	if err != nil {
		t.Fatal(err)
	}

	// Reload from disk
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.GetFeature("proj", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusWaiting {
		t.Errorf("status after reload: got %v, want waiting", got.Status)
	}
	if !got.DirectToBead {
		t.Error("DirectToBead not persisted: got false, want true")
	}
	if got.GenerateAfter != "dep-id" {
		t.Errorf("GenerateAfter after reload: got %q, want %q", got.GenerateAfter, "dep-id")
	}
}

// --- Concurrent access ---

func TestConcurrentCreateFeatures(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	n := 20
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.CreateFeature("p", "feat", "desc", false, "")
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent create error: %v", err)
	}

	features, err := s.ListFeatures("p", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(features) != n {
		t.Errorf("expected %d features, got %d", n, len(features))
	}

	// Check all IDs are unique
	seen := make(map[string]bool)
	for _, f := range features {
		if seen[f.ID] {
			t.Errorf("duplicate feature ID: %q", f.ID)
		}
		seen[f.ID] = true
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.ListFeatures("p", nil) //nolint
		}()
		go func() {
			defer wg.Done()
			s.GetFeature("p", f.ID) //nolint
		}()
	}
	wg.Wait()
}

// --- Dialog state machine ---

func setupDialogFeature(t *testing.T, status model.FeatureStatus) (*Store, *model.Feature) {
	t.Helper()
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != model.StatusDraft {
		s.mu.Lock()
		for i, feat := range s.features["p"] {
			if feat.ID == f.ID {
				s.features["p"][i].Status = status
			}
		}
		s.mu.Unlock()
		if err := s.saveFeatures("p"); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, updated
}

func TestStartDialog(t *testing.T) {
	s, f := setupDialogFeature(t, model.StatusDraft)
	if err := s.StartDialog("p", f.ID); err != nil {
		t.Fatalf("StartDialog: %v", err)
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusAwaitingClient {
		t.Errorf("expected awaiting_client, got %v", got.Status)
	}
}

func TestStartDialogWrongStatus(t *testing.T) {
	s, f := setupDialogFeature(t, model.StatusAwaitingClient)
	if err := s.StartDialog("p", f.ID); err == nil {
		t.Error("expected error when not in draft status")
	}
}

func TestStartDialogNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	if err := s.StartDialog("p", "no-such-id"); err == nil {
		t.Error("expected error for missing feature")
	}
}

func TestRespondToDialog(t *testing.T) {
	s, f := setupDialogFeature(t, model.StatusAwaitingHuman)
	// Set CurrentIteration to 1 so there's a round to respond to
	s.mu.Lock()
	for i, feat := range s.features["p"] {
		if feat.ID == f.ID {
			s.features["p"][i].CurrentIteration = 1
		}
	}
	s.mu.Unlock()
	if err := s.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	if err := s.RespondToDialog("p", f.ID, "my response", false); err != nil {
		t.Fatalf("RespondToDialog: %v", err)
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusAwaitingClient {
		t.Errorf("expected awaiting_client, got %v", got.Status)
	}
	// Verify file was written
	content, err := s.ReadResponse("p", f.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if content != "my response" {
		t.Errorf("response file: got %q, want %q", content, "my response")
	}
}

func TestRespondToDialogFinal(t *testing.T) {
	s, f := setupDialogFeature(t, model.StatusAwaitingHuman)
	s.mu.Lock()
	for i, feat := range s.features["p"] {
		if feat.ID == f.ID {
			s.features["p"][i].CurrentIteration = 2
		}
	}
	s.mu.Unlock()
	if err := s.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	if err := s.RespondToDialog("p", f.ID, "final answer", true); err != nil {
		t.Fatalf("RespondToDialog final: %v", err)
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusAwaitingClient {
		t.Errorf("expected awaiting_client, got %v", got.Status)
	}
	// Verify is_final is set
	found := false
	for _, it := range got.Iterations {
		if it.Round == 2 && it.IsFinal {
			found = true
		}
	}
	if !found {
		t.Errorf("expected is_final=true for round 2 in Iterations: %v", got.Iterations)
	}
}

func TestRespondToDialogFinalIdempotent(t *testing.T) {
	// Exercises the found=true branch: responding final=true when a DialogIteration
	// entry for this round already exists (e.g. the feature was reset to awaiting_human
	// via direct TransitionStatus after a previous final response).
	s, f := setupDialogFeature(t, model.StatusAwaitingHuman)
	s.mu.Lock()
	for i, feat := range s.features["p"] {
		if feat.ID == f.ID {
			s.features["p"][i].CurrentIteration = 1
		}
	}
	s.mu.Unlock()
	if err := s.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	// First final response — adds a new DialogIteration entry.
	if err := s.RespondToDialog("p", f.ID, "first final", true); err != nil {
		t.Fatalf("first RespondToDialog: %v", err)
	}

	// Reset to awaiting_human with the same CurrentIteration so we can call again.
	s.mu.Lock()
	for i, feat := range s.features["p"] {
		if feat.ID == f.ID {
			s.features["p"][i].Status = model.StatusAwaitingHuman
		}
	}
	s.mu.Unlock()
	if err := s.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	// Second final response — hits the found=true branch.
	if err := s.RespondToDialog("p", f.ID, "second final", true); err != nil {
		t.Fatalf("second RespondToDialog: %v", err)
	}

	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Should still have exactly one Iterations entry for round 1 with IsFinal=true.
	count := 0
	for _, it := range got.Iterations {
		if it.Round == 1 {
			count++
			if !it.IsFinal {
				t.Error("expected IsFinal=true on round 1 entry")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 DialogIteration for round 1, got %d: %v", count, got.Iterations)
	}
}

func TestRespondToDialogWrongStatus(t *testing.T) {
	s, f := setupDialogFeature(t, model.StatusAwaitingClient)
	if err := s.RespondToDialog("p", f.ID, "response", false); err == nil {
		t.Error("expected error when not in awaiting_human status")
	}
}

func TestRespondToDialogZeroIteration(t *testing.T) {
	// Feature in awaiting_human but CurrentIteration == 0 (no client round written yet)
	s, f := setupDialogFeature(t, model.StatusAwaitingHuman)
	// CurrentIteration defaults to 0 — should be rejected to avoid writing response_v0.md
	if err := s.RespondToDialog("p", f.ID, "response", false); err == nil {
		t.Error("expected error when current_iteration is 0")
	}
}

func TestReopenDialog(t *testing.T) {
	s, f := setupDialogFeature(t, model.StatusFullySpecified)
	// Set CurrentIteration to 2
	s.mu.Lock()
	for i, feat := range s.features["p"] {
		if feat.ID == f.ID {
			s.features["p"][i].CurrentIteration = 2
		}
	}
	s.mu.Unlock()
	if err := s.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	if err := s.ReopenDialog("p", f.ID, "please add X"); err != nil {
		t.Fatalf("ReopenDialog: %v", err)
	}
	got, err := s.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusAwaitingClient {
		t.Errorf("expected awaiting_client, got %v", got.Status)
	}
	if got.CurrentIteration != 3 {
		t.Errorf("expected CurrentIteration=3, got %d", got.CurrentIteration)
	}
	// Verify response file was written for the new round
	content, err := s.ReadResponse("p", f.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if content != "please add X" {
		t.Errorf("response file: got %q, want %q", content, "please add X")
	}
}

func TestReopenDialogWrongStatus(t *testing.T) {
	s, f := setupDialogFeature(t, model.StatusAwaitingHuman)
	if err := s.ReopenDialog("p", f.ID, "message"); err == nil {
		t.Error("expected error when not in fully_specified status")
	}
}

func TestRespondToDialogFinalPersistence(t *testing.T) {
	dir, err := os.MkdirTemp("", "bm-dialog-persist-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s1, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	f, err := s1.CreateFeature("p", "feat", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}

	// Force to awaiting_human with iteration=1
	s1.mu.Lock()
	for i, feat := range s1.features["p"] {
		if feat.ID == f.ID {
			s1.features["p"][i].Status = model.StatusAwaitingHuman
			s1.features["p"][i].CurrentIteration = 1
		}
	}
	s1.mu.Unlock()
	if err := s1.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	if err := s1.RespondToDialog("p", f.ID, "final answer", true); err != nil {
		t.Fatalf("RespondToDialog: %v", err)
	}

	// Reload from disk
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.GetFeature("p", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusAwaitingClient {
		t.Errorf("status after reload: got %v, want awaiting_client", got.Status)
	}
	found := false
	for _, it := range got.Iterations {
		if it.Round == 1 && it.IsFinal {
			found = true
		}
	}
	if !found {
		t.Errorf("is_final not persisted after reload: Iterations=%v", got.Iterations)
	}
}

// --- ClaimFeature ---

func TestClaimFeature_NoActionable(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	// No features at all: returns nil
	f, _, err := s.ClaimFeature("p")
	if err != nil {
		t.Fatalf("ClaimFeature: %v", err)
	}
	if f != nil {
		t.Errorf("expected nil feature, got %+v", f)
	}
}

func TestClaimFeature_AwaitingClient(t *testing.T) {
	s, feat := setupDialogFeature(t, model.StatusAwaitingClient)
	f, action, err := s.ClaimFeature("p")
	if err != nil {
		t.Fatalf("ClaimFeature: %v", err)
	}
	if f == nil {
		t.Fatal("expected feature, got nil")
	}
	if f.ID != feat.ID {
		t.Errorf("feature ID: got %q, want %q", f.ID, feat.ID)
	}
	if action != model.ActionDialogStep {
		t.Errorf("action: got %v, want dialog_step", action)
	}
	if f.ClaimedAt == nil {
		t.Error("ClaimedAt should be set after claim")
	}
}

func TestClaimFeature_ReadyToGenerate(t *testing.T) {
	s, feat := setupDialogFeature(t, model.StatusReadyToGenerate)
	f, action, err := s.ClaimFeature("p")
	if err != nil {
		t.Fatalf("ClaimFeature: %v", err)
	}
	if f == nil {
		t.Fatal("expected feature, got nil")
	}
	if f.ID != feat.ID {
		t.Errorf("feature ID: got %q, want %q", f.ID, feat.ID)
	}
	if action != model.ActionGenerate {
		t.Errorf("action: got %v, want generate", action)
	}
	if f.ClaimedAt == nil {
		t.Error("ClaimedAt should be set after claim")
	}
}

func TestClaimFeature_SkipsClaimed(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProject("p", "tok"); err != nil {
		t.Fatal(err)
	}
	// Create two awaiting_client features.
	f1, err := s.CreateFeature("p", "feat1", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := s.CreateFeature("p", "feat2", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	// Manually put both into awaiting_client.
	s.mu.Lock()
	for i := range s.features["p"] {
		s.features["p"][i].Status = model.StatusAwaitingClient
	}
	s.mu.Unlock()
	if err := s.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	// First claim should get f1.
	got1, _, err := s.ClaimFeature("p")
	if err != nil {
		t.Fatal(err)
	}
	if got1 == nil || got1.ID != f1.ID {
		t.Fatalf("first claim: expected %q, got %v", f1.ID, got1)
	}

	// Second claim should get f2 (f1 is already claimed).
	got2, _, err := s.ClaimFeature("p")
	if err != nil {
		t.Fatal(err)
	}
	if got2 == nil || got2.ID != f2.ID {
		t.Fatalf("second claim: expected %q, got %v", f2.ID, got2)
	}

	// Third claim should return nil (both claimed).
	got3, _, err := s.ClaimFeature("p")
	if err != nil {
		t.Fatal(err)
	}
	if got3 != nil {
		t.Errorf("third claim: expected nil, got %+v", got3)
	}
}

func TestClaimFeature_ExpiredClaim(t *testing.T) {
	s, _ := setupDialogFeature(t, model.StatusAwaitingClient)

	// Set ClaimedAt to an expired time.
	expired := time.Now().Add(-(ClaimTTL + time.Second))
	s.mu.Lock()
	for i := range s.features["p"] {
		s.features["p"][i].ClaimedAt = &expired
	}
	s.mu.Unlock()
	if err := s.saveFeatures("p"); err != nil {
		t.Fatal(err)
	}

	// Claim should succeed because the previous claim has expired.
	f, _, err := s.ClaimFeature("p")
	if err != nil {
		t.Fatalf("ClaimFeature: %v", err)
	}
	if f == nil {
		t.Fatal("expected feature for expired claim, got nil")
	}
	if f.ClaimedAt == nil || !f.ClaimedAt.After(expired) {
		t.Error("ClaimedAt should be updated to a fresh timestamp")
	}
}

func TestClaimFeature_ConcurrentOnlyOneWins(t *testing.T) {
	s, _ := setupDialogFeature(t, model.StatusReadyToGenerate)

	n := 20
	results := make([]*model.Feature, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f, _, err := s.ClaimFeature("p")
			if err != nil {
				t.Errorf("goroutine %d: ClaimFeature error: %v", i, err)
				return
			}
			results[i] = f
		}(i)
	}
	wg.Wait()

	winners := 0
	for _, r := range results {
		if r != nil {
			winners++
		}
	}
	if winners != 1 {
		t.Errorf("expected exactly 1 winner, got %d", winners)
	}
}

func TestTransitionStatus_ClearsClaimedAt(t *testing.T) {
	s, feat := setupDialogFeature(t, model.StatusReadyToGenerate)

	// Claim the feature.
	f, _, err := s.ClaimFeature("p")
	if err != nil || f == nil {
		t.Fatalf("ClaimFeature: %v (feature=%v)", err, f)
	}

	// Transition clears ClaimedAt.
	if err := s.TransitionStatus("p", feat.ID, model.StatusGenerating); err != nil {
		t.Fatalf("TransitionStatus: %v", err)
	}
	got, err := s.GetFeature("p", feat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClaimedAt != nil {
		t.Errorf("ClaimedAt should be nil after transition, got %v", got.ClaimedAt)
	}
}

func TestSubmitClientDialog_ClearsClaimedAt(t *testing.T) {
	s, feat := setupDialogFeature(t, model.StatusAwaitingClient)

	// Claim the feature.
	f, _, err := s.ClaimFeature("p")
	if err != nil || f == nil {
		t.Fatalf("ClaimFeature: %v (feature=%v)", err, f)
	}

	// SubmitClientDialog clears ClaimedAt.
	if err := s.SubmitClientDialog("p", feat.ID, "updated desc", "some questions"); err != nil {
		t.Fatalf("SubmitClientDialog: %v", err)
	}
	got, err := s.GetFeature("p", feat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClaimedAt != nil {
		t.Errorf("ClaimedAt should be nil after SubmitClientDialog, got %v", got.ClaimedAt)
	}
}
