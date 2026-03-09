package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vector76/backlog_manager/internal/beadsserver"
	"github.com/vector76/backlog_manager/internal/model"
	"github.com/vector76/backlog_manager/internal/server"
	"github.com/vector76/backlog_manager/internal/store"
)

// newMonitorStore sets up a test store with a project and feature in beads_created status.
func newMonitorStore(t *testing.T, beadIDs []string) (*store.Store, string, string) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj, err := st.CreateProject("testproject", "token123")
	if err != nil {
		t.Fatal(err)
	}
	feat, err := st.CreateFeature(proj.Name, "My Feature", "description", false, "")
	if err != nil {
		t.Fatal(err)
	}
	// Advance through state machine to beads_created.
	for _, s := range []model.FeatureStatus{
		model.StatusAwaitingClient,
		model.StatusFullySpecified,
		model.StatusReadyToGenerate,
		model.StatusGenerating,
	} {
		if err := st.TransitionStatus(proj.Name, feat.ID, s); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	// Set BeadIDs and transition to beads_created.
	f, err := st.GetFeature(proj.Name, feat.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.BeadIDs = beadIDs
	if err := st.UpdateFeature(f); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(proj.Name, feat.ID, model.StatusBeadsCreated); err != nil {
		t.Fatal(err)
	}
	return st, proj.Name, feat.ID
}

// mockBeadsServer creates an httptest server returning the given statuses.
func mockBeadsServer(t *testing.T, statuses map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statuses)
	}))
}

func TestBeadMonitor_poll_partialClosed(t *testing.T) {
	beadIDs := []string{"bd-a1", "bd-a2", "bd-a3"}
	st, projName, featureID := newMonitorStore(t, beadIDs)

	statuses := map[string]string{
		"bd-a1": "closed",
		"bd-a2": "in_progress",
		"bd-a3": "open",
	}
	srv := mockBeadsServer(t, statuses)
	defer srv.Close()

	client := beadsserver.New(srv.URL)
	monitor := server.NewBeadMonitor(client, st, time.Hour)
	monitor.Poll()

	progress, ok := monitor.GetProgress(featureID)
	if !ok {
		t.Fatal("expected progress to be cached")
	}
	if progress.Total != 3 {
		t.Errorf("total: want 3, got %d", progress.Total)
	}
	if progress.Closed != 1 {
		t.Errorf("closed: want 1, got %d", progress.Closed)
	}
	if progress.Unavailable {
		t.Error("expected Unavailable=false")
	}

	// Feature should still be in beads_created.
	f, err := st.GetFeature(projName, featureID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusBeadsCreated {
		t.Errorf("status: want beads_created, got %s", f.Status)
	}
}

func TestBeadMonitor_poll_allClosed_autoComplete(t *testing.T) {
	beadIDs := []string{"bd-b1", "bd-b2"}
	st, projName, featureID := newMonitorStore(t, beadIDs)

	statuses := map[string]string{
		"bd-b1": "closed",
		"bd-b2": "closed",
	}
	srv := mockBeadsServer(t, statuses)
	defer srv.Close()

	client := beadsserver.New(srv.URL)
	monitor := server.NewBeadMonitor(client, st, time.Hour)
	monitor.Poll()

	// Feature should have been auto-completed.
	f, err := st.GetFeature(projName, featureID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusDone {
		t.Errorf("status: want done, got %s", f.Status)
	}
}

func TestBeadMonitor_poll_allClosed_triggersDependencyResolution(t *testing.T) {
	beadIDs := []string{"bd-c1"}
	st, projName, featureID := newMonitorStore(t, beadIDs)

	// Create a second feature in waiting state that depends on the first.
	waitFeat, err := st.CreateFeature(projName, "Dependent Feature", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []model.FeatureStatus{
		model.StatusAwaitingClient,
		model.StatusFullySpecified,
	} {
		if err := st.TransitionStatus(projName, waitFeat.ID, s); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	wf, err := st.GetFeature(projName, waitFeat.ID)
	if err != nil {
		t.Fatal(err)
	}
	wf.GenerateAfter = featureID
	if err := st.UpdateFeature(wf); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(projName, waitFeat.ID, model.StatusWaiting); err != nil {
		t.Fatal(err)
	}

	srv := mockBeadsServer(t, map[string]string{"bd-c1": "closed"})
	defer srv.Close()

	client := beadsserver.New(srv.URL)
	monitor := server.NewBeadMonitor(client, st, time.Hour)
	monitor.Poll()

	// First feature should be done.
	f, err := st.GetFeature(projName, featureID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusDone {
		t.Errorf("first feature: want done, got %s", f.Status)
	}

	// Dependent feature should be unblocked to ready_to_generate.
	dep, err := st.GetFeature(projName, waitFeat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Status != model.StatusReadyToGenerate {
		t.Errorf("dependent feature: want ready_to_generate, got %s", dep.Status)
	}
}

func TestBeadMonitor_poll_beadsCreated_doesNotUnblockDependent(t *testing.T) {
	beadIDs := []string{"bd-e1"}
	st, projName, featureID := newMonitorStore(t, beadIDs)

	// Create a dependent feature in waiting state.
	waitFeat, err := st.CreateFeature(projName, "Dependent Feature", "desc", false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []model.FeatureStatus{
		model.StatusAwaitingClient,
		model.StatusFullySpecified,
	} {
		if err := st.TransitionStatus(projName, waitFeat.ID, s); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	wf, err := st.GetFeature(projName, waitFeat.ID)
	if err != nil {
		t.Fatal(err)
	}
	wf.GenerateAfter = featureID
	if err := st.UpdateFeature(wf); err != nil {
		t.Fatal(err)
	}
	if err := st.TransitionStatus(projName, waitFeat.ID, model.StatusWaiting); err != nil {
		t.Fatal(err)
	}

	// Bead is still open (not closed), so provider should stay in beads_created.
	srv := mockBeadsServer(t, map[string]string{"bd-e1": "open"})
	defer srv.Close()

	client := beadsserver.New(srv.URL)
	monitor := server.NewBeadMonitor(client, st, time.Hour)
	monitor.Poll()

	// First feature must remain in beads_created.
	f, err := st.GetFeature(projName, featureID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusBeadsCreated {
		t.Errorf("first feature: want beads_created, got %s", f.Status)
	}

	// Dependent feature must remain in waiting.
	dep, err := st.GetFeature(projName, waitFeat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Status != model.StatusWaiting {
		t.Errorf("dependent feature: want waiting, got %s", dep.Status)
	}
}

func TestBeadMonitor_poll_serverDown_gracefulDegradation(t *testing.T) {
	beadIDs := []string{"bd-d1", "bd-d2"}
	st, _, featureID := newMonitorStore(t, beadIDs)

	// Use an unreachable server.
	client := beadsserver.New("http://127.0.0.1:1")
	monitor := server.NewBeadMonitor(client, st, time.Hour)
	monitor.Poll() // Should not panic or crash.

	// Progress should be marked unavailable.
	progress, ok := monitor.GetProgress(featureID)
	if !ok {
		t.Fatal("expected cached progress even on failure")
	}
	if !progress.Unavailable {
		t.Error("expected Unavailable=true when server is down")
	}
	if progress.Total != 2 {
		t.Errorf("total: want 2, got %d", progress.Total)
	}
}

// errClient always returns an error for GetStatuses.
type errClient struct{ err error }

func (e *errClient) GetStatuses(_ []string) (map[string]string, error) {
	return nil, e.err
}

func (e *errClient) SubscribeSSE(_ context.Context) <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestBeadMonitor_poll_clientError_doesNotCrash(t *testing.T) {
	beadIDs := []string{"bd-e1"}
	st, _, _ := newMonitorStore(t, beadIDs)

	client := &errClient{err: errors.New("connection refused")}
	monitor := server.NewBeadMonitor(client, st, time.Hour)
	// Should not panic.
	monitor.Poll()
}

func TestBeadMonitor_poll_zeroBeads_autoComplete(t *testing.T) {
	// A feature in beads_created with no beads should be auto-completed immediately.
	st, projName, featureID := newMonitorStore(t, []string{})

	// No bead server needed — zero beads means no external calls.
	client := &errClient{err: errors.New("should not be called")}
	monitor := server.NewBeadMonitor(client, st, time.Hour)
	monitor.Poll()

	f, err := st.GetFeature(projName, featureID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusDone {
		t.Errorf("zero-bead feature: want done after poll, got %s", f.Status)
	}
}

func TestBeadMonitor_setNotify_calledOnCountChange(t *testing.T) {
	beadIDs := []string{"bd-f1", "bd-f2", "bd-f3"}
	st, _, featureID := newMonitorStore(t, beadIDs)

	// First poll: 1 closed.
	srv1 := mockBeadsServer(t, map[string]string{
		"bd-f1": "closed",
		"bd-f2": "open",
		"bd-f3": "open",
	})
	defer srv1.Close()

	client := beadsserver.New(srv1.URL)
	monitor := server.NewBeadMonitor(client, st, time.Hour)

	var notified []string
	monitor.SetNotify(func(proj, feat string) {
		notified = append(notified, proj+":"+feat)
	})

	monitor.Poll() // closed goes 0→1
	if len(notified) != 1 {
		t.Fatalf("after first poll: want 1 notify, got %d", len(notified))
	}
	if notified[0] != "testproject:"+featureID {
		t.Errorf("notify key: want testproject:%s, got %s", featureID, notified[0])
	}

	// Second poll with same count: no notify.
	notified = nil
	monitor.Poll()
	if len(notified) != 0 {
		t.Errorf("second poll with no change: want 0 notifies, got %d", len(notified))
	}
}

func TestBeadMonitor_setNotify_calledOnAutoComplete(t *testing.T) {
	beadIDs := []string{"bd-g1", "bd-g2"}
	st, projName, featureID := newMonitorStore(t, beadIDs)

	srv := mockBeadsServer(t, map[string]string{
		"bd-g1": "closed",
		"bd-g2": "closed",
	})
	defer srv.Close()

	client := beadsserver.New(srv.URL)
	monitor := server.NewBeadMonitor(client, st, time.Hour)

	var notifyCount int
	monitor.SetNotify(func(proj, feat string) {
		notifyCount++
	})

	monitor.Poll() // all closed → notify for count change + notify for auto-complete

	// At least the auto-complete notify must fire.
	if notifyCount < 1 {
		t.Fatalf("want at least 1 notify on auto-complete, got %d", notifyCount)
	}

	// Feature must be done.
	f, err := st.GetFeature(projName, featureID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != model.StatusDone {
		t.Errorf("status: want done, got %s", f.Status)
	}
}
