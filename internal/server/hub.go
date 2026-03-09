package server

import (
	"context"
	"sync"
	"time"
)

// NotifyHub manages SSE subscribers and broadcasts events to them.
// It does not carry data payloads — it only signals "something may have changed."
type NotifyHub struct {
	mu           sync.Mutex
	dash         map[chan struct{}]struct{}
	features     map[string]map[chan struct{}]struct{}
	tickInterval time.Duration
	stop         chan struct{}
	stopOnce     sync.Once
}

// NewNotifyHub creates a hub with a 30-second tick interval.
func NewNotifyHub() *NotifyHub {
	return NewNotifyHubWithInterval(30 * time.Second)
}

// NewNotifyHubWithInterval creates a hub with a custom tick interval (for testing).
func NewNotifyHubWithInterval(tickInterval time.Duration) *NotifyHub {
	return &NotifyHub{
		dash:         make(map[chan struct{}]struct{}),
		features:     make(map[string]map[chan struct{}]struct{}),
		tickInterval: tickInterval,
		stop:         make(chan struct{}),
	}
}

// Start launches a background goroutine that broadcasts every tickInterval.
// It stops when ctx is done or the stop channel closes.
func (h *NotifyHub) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(h.tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.NotifyDashboard()
			case <-ctx.Done():
				return
			case <-h.stop:
				return
			}
		}
	}()
}

// Stop closes the stop channel for clean shutdown. Safe to call multiple times.
func (h *NotifyHub) Stop() {
	h.stopOnce.Do(func() { close(h.stop) })
}

// SubscribeDashboard creates a buffered chan, adds to dash set, returns it.
func (h *NotifyHub) SubscribeDashboard() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.dash[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// UnsubscribeDashboard removes the channel from the dash set.
func (h *NotifyHub) UnsubscribeDashboard(ch chan struct{}) {
	h.mu.Lock()
	delete(h.dash, ch)
	h.mu.Unlock()
}

// SubscribeFeature creates a buffered chan, adds to features[key] set, returns it.
func (h *NotifyHub) SubscribeFeature(key string) chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	if h.features[key] == nil {
		h.features[key] = make(map[chan struct{}]struct{})
	}
	h.features[key][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// UnsubscribeFeature removes the channel from features[key] set.
func (h *NotifyHub) UnsubscribeFeature(key string, ch chan struct{}) {
	h.mu.Lock()
	if h.features[key] != nil {
		delete(h.features[key], ch)
	}
	h.mu.Unlock()
}

// NotifyDashboard sends to all dash subscriber channels non-blocking.
func (h *NotifyHub) NotifyDashboard() {
	h.mu.Lock()
	channels := make([]chan struct{}, 0, len(h.dash))
	for ch := range h.dash {
		channels = append(channels, ch)
	}
	h.mu.Unlock()

	for _, ch := range channels {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// NotifyFeature sends to all features[key] subscriber channels non-blocking,
// and also calls NotifyDashboard().
func (h *NotifyHub) NotifyFeature(key string) {
	h.mu.Lock()
	var channels []chan struct{}
	if h.features[key] != nil {
		channels = make([]chan struct{}, 0, len(h.features[key]))
		for ch := range h.features[key] {
			channels = append(channels, ch)
		}
	}
	h.mu.Unlock()

	for _, ch := range channels {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	h.NotifyDashboard()
}
