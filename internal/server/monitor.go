package server

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/vector76/backlog_manager/internal/model"
)

// BeadsClient is the interface for querying bead statuses from the beads server.
type BeadsClient interface {
	GetStatuses(ids []string) (map[string]string, error)

	// SubscribeSSE opens an SSE stream and returns a receive-only channel.
	// One send occurs per "data:" line received from the server.
	// The channel is closed when the connection drops (EOF or error).
	// The caller cancels the stream via the provided context.
	SubscribeSSE(ctx context.Context) <-chan struct{}
}

// BeadProgress holds the aggregated bead completion progress for a feature.
type BeadProgress struct {
	Total       int
	Closed      int
	Statuses    map[string]string // beadID -> status
	Unavailable bool
}

// BeadMonitor polls the beads server periodically for features in beads_created status,
// caches progress, and automatically transitions features to done when all beads are closed.
type BeadMonitor struct {
	client   BeadsClient
	store    Store
	interval time.Duration
	notify   func(projectName, featureID string)

	mu    sync.RWMutex
	cache map[string]BeadProgress // featureID -> progress
}

// SetNotify sets a callback that is invoked when a feature's bead state changes.
func (m *BeadMonitor) SetNotify(fn func(string, string)) {
	m.notify = fn
}

// NewBeadMonitor creates a BeadMonitor with the given client, store, and poll interval.
func NewBeadMonitor(client BeadsClient, st Store, interval time.Duration) *BeadMonitor {
	return &BeadMonitor{
		client:   client,
		store:    st,
		interval: interval,
		cache:    make(map[string]BeadProgress),
	}
}

// Start begins polling in a background goroutine.
func (m *BeadMonitor) Start() {
	go m.run()
}

func (m *BeadMonitor) run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for range ticker.C {
		m.Poll()
	}
}

// Poll runs a single polling cycle across all projects. It is exported for testing.
func (m *BeadMonitor) Poll() {
	projects := m.store.ListProjects()
	for _, p := range projects {
		status := model.StatusBeadsCreated
		features, err := m.store.ListFeatures(p.Name, &status)
		if err != nil {
			continue
		}
		for _, f := range features {
			if len(f.BeadIDs) == 0 {
				// Zero beads: nothing to wait for, auto-complete immediately.
				if err := m.store.TransitionStatus(p.Name, f.ID, model.StatusDone); err != nil {
					log.Printf("bead monitor: auto-complete zero-bead feature %s: %v", f.ID, err)
					continue
				}
				log.Printf("bead monitor: auto-completed feature %s (zero beads)", f.ID)
				if allFeatures, err := m.store.ListFeatures(p.Name, nil); err == nil {
					for _, dep := range allFeatures {
						if dep.Status == model.StatusWaiting && dep.GenerateAfter == f.ID {
							_ = m.store.TransitionStatus(p.Name, dep.ID, model.StatusReadyToGenerate)
						}
					}
				}
				continue
			}
			m.checkFeature(p.Name, f)
		}
	}
}

func (m *BeadMonitor) checkFeature(projectName string, f model.Feature) {
	statuses, err := m.client.GetStatuses(f.BeadIDs)
	if err != nil {
		log.Printf("bead monitor: warning: could not get statuses for feature %s: %v", f.ID, err)
		m.mu.Lock()
		m.cache[f.ID] = BeadProgress{
			Total:       len(f.BeadIDs),
			Unavailable: true,
		}
		m.mu.Unlock()
		return
	}

	closed := 0
	for _, id := range f.BeadIDs {
		if statuses[id] == "closed" {
			closed++
		}
	}

	m.mu.Lock()
	prev := m.cache[f.ID]
	m.cache[f.ID] = BeadProgress{
		Total:    len(f.BeadIDs),
		Closed:   closed,
		Statuses: statuses,
	}
	m.mu.Unlock()

	if m.notify != nil && closed != prev.Closed {
		m.notify(projectName, f.ID)
	}

	// Auto-complete if all beads are closed.
	if closed == len(f.BeadIDs) {
		if err := m.store.TransitionStatus(projectName, f.ID, model.StatusDone); err != nil {
			log.Printf("bead monitor: auto-complete feature %s: %v", f.ID, err)
			return
		}
		log.Printf("bead monitor: auto-completed feature %s (all %d beads closed)", f.ID, closed)
		if m.notify != nil {
			m.notify(projectName, f.ID)
		}
		// Dependency resolution: unblock waiting features that depended on this one.
		if allFeatures, err := m.store.ListFeatures(projectName, nil); err == nil {
			for _, dep := range allFeatures {
				if dep.Status == model.StatusWaiting && dep.GenerateAfter == f.ID {
					_ = m.store.TransitionStatus(projectName, dep.ID, model.StatusReadyToGenerate)
				}
			}
		}
	}
}

// GetProgress returns the cached bead progress for the given feature ID.
// Returns false if no progress data is cached.
func (m *BeadMonitor) GetProgress(featureID string) (BeadProgress, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.cache[featureID]
	return p, ok
}
