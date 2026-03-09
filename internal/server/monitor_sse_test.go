package server_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vector76/backlog_manager/internal/server"
)

// sseControlClient is a BeadsClient mock for SSE control flow tests.
type sseControlClient struct {
	getStatusesCalls chan struct{}
	sseCh            chan struct{}
}

func (c *sseControlClient) GetStatuses(ids []string) (map[string]string, error) {
	select {
	case c.getStatusesCalls <- struct{}{}:
	default:
	}
	return map[string]string{}, nil
}

func (c *sseControlClient) SubscribeSSE(ctx context.Context) <-chan struct{} {
	return c.sseCh
}

// multiSSEClient returns successive SSE channels per SubscribeSSE call.
type multiSSEClient struct {
	getStatusesCalls chan struct{}
	sseChannels      []chan struct{}
	mu               sync.Mutex
	idx              int
}

func (c *multiSSEClient) GetStatuses(ids []string) (map[string]string, error) {
	select {
	case c.getStatusesCalls <- struct{}{}:
	default:
	}
	return map[string]string{}, nil
}

func (c *multiSSEClient) SubscribeSSE(ctx context.Context) <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := c.sseChannels[c.idx]
	if c.idx < len(c.sseChannels)-1 {
		c.idx++
	}
	return ch
}

func TestBeadMonitor_sseEventTriggersPoll(t *testing.T) {
	st, _, _ := newMonitorStore(t, []string{"bd-sse1"})
	sseCh := make(chan struct{}, 1)
	client := &sseControlClient{
		getStatusesCalls: make(chan struct{}, 10),
		sseCh:            sseCh,
	}
	monitor := server.NewBeadMonitor(client, st, time.Second)
	monitor.Start()
	defer monitor.Stop()

	// Drain the initial poll.
	select {
	case <-client.getStatusesCalls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial poll")
	}

	// Send one SSE event.
	sseCh <- struct{}{}

	// Verify a second poll is triggered.
	select {
	case <-client.getStatusesCalls:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for SSE-triggered poll")
	}
}

func TestBeadMonitor_sseDropCausesFallback(t *testing.T) {
	st, _, _ := newMonitorStore(t, []string{"bd-sse2"})
	sseCh := make(chan struct{}, 1)
	client := &sseControlClient{
		getStatusesCalls: make(chan struct{}, 10),
		sseCh:            sseCh,
	}
	monitor := server.NewBeadMonitor(client, st, 20*time.Millisecond)
	monitor.Start()
	defer monitor.Stop()

	// Drain the initial poll.
	select {
	case <-client.getStatusesCalls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial poll")
	}

	// Deliver one event then drop the SSE connection.
	sseCh <- struct{}{}
	close(sseCh)

	// After the drop, GetStatuses must continue at the fallback interval.
	for i := 0; i < 2; i++ {
		select {
		case <-client.getStatusesCalls:
			// good
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for fallback poll #%d", i+1)
		}
	}
}

func TestBeadMonitor_initialPollRunsBeforeSSEEvents(t *testing.T) {
	st, _, _ := newMonitorStore(t, []string{"bd-sse3"})
	sseCh := make(chan struct{}) // never written to — blocks forever
	client := &sseControlClient{
		getStatusesCalls: make(chan struct{}, 1),
		sseCh:            sseCh,
	}
	monitor := server.NewBeadMonitor(client, st, time.Second)
	monitor.Start()
	defer monitor.Stop()

	// Initial poll must fire before any SSE event arrives.
	select {
	case <-client.getStatusesCalls:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial poll")
	}
}

func TestBeadMonitor_sseReconnectOnFallbackTick(t *testing.T) {
	st, _, _ := newMonitorStore(t, []string{"bd-sse4"})

	closedCh := make(chan struct{})
	close(closedCh) // first SSE attempt fails immediately
	liveCh := make(chan struct{}, 1)

	client := &multiSSEClient{
		getStatusesCalls: make(chan struct{}, 10),
		sseChannels:      []chan struct{}{closedCh, liveCh},
	}
	monitor := server.NewBeadMonitor(client, st, 20*time.Millisecond)
	monitor.Start()
	defer monitor.Stop()

	// Drain initial poll.
	select {
	case <-client.getStatusesCalls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial poll")
	}

	// Drain poll from first fallback tick (reconnect to liveCh succeeds here).
	select {
	case <-client.getStatusesCalls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for fallback tick poll")
	}

	// After reconnect, no further polls without SSE events (wait 3× the interval).
	select {
	case <-client.getStatusesCalls:
		t.Fatal("unexpected poll without SSE event after reconnect")
	case <-time.After(60 * time.Millisecond):
		// good — no spurious polls
	}

	// Sending an event on the live channel must trigger a poll.
	liveCh <- struct{}{}
	select {
	case <-client.getStatusesCalls:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for poll after SSE event on reconnected channel")
	}
}

func TestBeadMonitor_sseNeverConnects(t *testing.T) {
	st, _, _ := newMonitorStore(t, []string{"bd-sse5"})
	sseCh := make(chan struct{})
	close(sseCh) // pre-closed — SSE never connects
	client := &sseControlClient{
		getStatusesCalls: make(chan struct{}, 10),
		sseCh:            sseCh,
	}
	monitor := server.NewBeadMonitor(client, st, 20*time.Millisecond)
	monitor.Start()
	defer monitor.Stop()

	// Drain initial poll.
	select {
	case <-client.getStatusesCalls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for initial poll")
	}

	// GetStatuses must be called repeatedly at the fallback interval.
	for i := 0; i < 2; i++ {
		select {
		case <-client.getStatusesCalls:
			// good
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for fallback poll #%d", i+1)
		}
	}
}

func TestBeadMonitor_stopCancelsSSESubscription(t *testing.T) {
	st, _, _ := newMonitorStore(t, []string{"bd-sse6"})
	sseCh := make(chan struct{}, 1)
	client := &sseControlClient{
		getStatusesCalls: make(chan struct{}, 1),
		sseCh:            sseCh,
	}
	monitor := server.NewBeadMonitor(client, st, time.Second)
	monitor.Start()

	// Stop must complete without hanging.
	monitor.Stop()
}
