package server

import (
	"testing"
	"time"

	"github.com/vector76/backlog_manager/internal/model"
	"github.com/vector76/backlog_manager/internal/store"
)

// setupBeadsCreatedFeature creates a store with a beads_created feature that has 4 bead IDs.
func setupBeadsCreatedFeature(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if _, err := st.CreateProject("proj", "tok"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	f, err := st.CreateFeature("proj", "feat", "desc", false, "")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	// Advance through state machine to generating.
	for _, s := range []model.FeatureStatus{
		model.StatusAwaitingClient,
		model.StatusFullySpecified,
		model.StatusReadyToGenerate,
		model.StatusGenerating,
	} {
		if err := st.TransitionStatus("proj", f.ID, s); err != nil {
			t.Fatalf("TransitionStatus to %v: %v", s, err)
		}
	}
	// Append bead IDs while in generating status.
	for _, id := range []string{"bd-1", "bd-2", "bd-3", "bd-4"} {
		if err := st.AppendBeadID("proj", f.ID, id); err != nil {
			t.Fatalf("AppendBeadID %s: %v", id, err)
		}
	}
	if err := st.TransitionStatus("proj", f.ID, model.StatusBeadsCreated); err != nil {
		t.Fatalf("TransitionStatus to beads_created: %v", err)
	}
	return st, "proj", f.ID
}

func TestBuildFeatureDetailData_beadsCreated_nilMonitor(t *testing.T) {
	st, projectName, featureID := setupBeadsCreatedFeature(t)
	data, err := buildFeatureDetailData(st, nil, projectName, featureID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.BeadProgress == nil {
		t.Fatal("expected BeadProgress to be non-nil")
	}
	if data.BeadProgress.Total != 4 {
		t.Errorf("Total = %d, want 4", data.BeadProgress.Total)
	}
	if data.BeadProgress.Closed != 0 {
		t.Errorf("Closed = %d, want 0", data.BeadProgress.Closed)
	}
	if data.BeadProgress.Unavailable {
		t.Error("Unavailable = true, want false")
	}
}

func TestBuildFeatureDetailData_beadsCreated_cacheMiss(t *testing.T) {
	st, projectName, featureID := setupBeadsCreatedFeature(t)
	monitor := NewBeadMonitor(nil, nil, time.Hour)
	data, err := buildFeatureDetailData(st, monitor, projectName, featureID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.BeadProgress == nil {
		t.Fatal("expected BeadProgress to be non-nil")
	}
	if data.BeadProgress.Total != 4 {
		t.Errorf("Total = %d, want 4", data.BeadProgress.Total)
	}
	if data.BeadProgress.Closed != 0 {
		t.Errorf("Closed = %d, want 0", data.BeadProgress.Closed)
	}
	if data.BeadProgress.Unavailable {
		t.Error("Unavailable = true, want false")
	}
}

func TestBuildFeatureDetailData_beadsCreated_unavailable(t *testing.T) {
	st, projectName, featureID := setupBeadsCreatedFeature(t)
	monitor := makeMonitorWithCache(map[string]BeadProgress{
		featureID: {Total: 4, Unavailable: true},
	})
	data, err := buildFeatureDetailData(st, monitor, projectName, featureID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.BeadProgress == nil {
		t.Fatal("expected BeadProgress to be non-nil")
	}
	if !data.BeadProgress.Unavailable {
		t.Error("Unavailable = false, want true")
	}
	if data.BeadProgress.Total != 4 {
		t.Errorf("Total = %d, want 4", data.BeadProgress.Total)
	}
}
