package server

import (
	"testing"
	"time"
)

// makeMonitorWithCache creates a BeadMonitor with pre-populated cache entries.
func makeMonitorWithCache(entries map[string]BeadProgress) *BeadMonitor {
	m := NewBeadMonitor(nil, nil, time.Hour)
	m.mu.Lock()
	for k, v := range entries {
		m.cache[k] = v
	}
	m.mu.Unlock()
	return m
}

func TestBeadInfoString_nilMonitor(t *testing.T) {
	got := beadInfoString("ft-1", nil, 7)
	want := "0/7 beads closed"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBeadInfoString_cacheMiss(t *testing.T) {
	monitor := NewBeadMonitor(nil, nil, time.Hour)
	got := beadInfoString("ft-1", monitor, 5)
	want := "0/5 beads closed"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBeadInfoString_unavailable(t *testing.T) {
	monitor := makeMonitorWithCache(map[string]BeadProgress{
		"ft-1": {Total: 3, Unavailable: true},
	})
	got := beadInfoString("ft-1", monitor, 3)
	want := "?/3 beads closed"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBeadInfoString_normal(t *testing.T) {
	monitor := makeMonitorWithCache(map[string]BeadProgress{
		"ft-1": {Total: 5, Closed: 2},
	})
	got := beadInfoString("ft-1", monitor, 5)
	want := "2/5 beads closed"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
