package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/vector76/backlog_manager/internal/server"
)

func TestHubBroadcastNotifiesSubscribers(t *testing.T) {
	h := server.NewNotifyHub()

	ch1 := h.SubscribeDashboard()
	ch2 := h.SubscribeDashboard()
	ch3 := h.SubscribeDashboard()

	h.NotifyDashboard()

	timeout := time.After(100 * time.Millisecond)
	for _, ch := range []chan struct{}{ch1, ch2, ch3} {
		select {
		case <-ch:
		case <-timeout:
			t.Fatal("timed out waiting for notification")
		}
	}
}

func TestHubUnsubscribeReceivesNothing(t *testing.T) {
	h := server.NewNotifyHub()

	ch := h.SubscribeDashboard()
	h.UnsubscribeDashboard(ch)

	h.NotifyDashboard()

	select {
	case <-ch:
		t.Fatal("unsubscribed channel should not receive notification")
	case <-time.After(50 * time.Millisecond):
		// expected: nothing received
	}
}

func TestHubStalledSubscriberDoesNotBlock(t *testing.T) {
	h := server.NewNotifyHub()

	// Fill the stalled subscriber's buffer
	stalled := h.SubscribeDashboard()
	stalled <- struct{}{} // buffer is 1, now full

	normal := h.SubscribeDashboard()

	h.NotifyDashboard()

	select {
	case <-normal:
		// expected: normal channel received
	case <-time.After(100 * time.Millisecond):
		t.Fatal("normal channel timed out — stalled subscriber may have blocked")
	}
}

func TestHubFeatureNotifiesFeatureAndDashboard(t *testing.T) {
	h := server.NewNotifyHub()

	featureCh := h.SubscribeFeature("feature-1")
	dashCh := h.SubscribeDashboard()

	h.NotifyFeature("feature-1")

	timeout := time.After(100 * time.Millisecond)
	for _, ch := range []chan struct{}{featureCh, dashCh} {
		select {
		case <-ch:
		case <-timeout:
			t.Fatal("timed out waiting for notification")
		}
	}
}

func TestHubPeriodicTick(t *testing.T) {
	h := server.NewNotifyHubWithInterval(10 * time.Millisecond)

	ch := h.SubscribeDashboard()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.Start(ctx)

	select {
	case <-ch:
		// expected: periodic tick notified subscriber
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for periodic tick")
	}
}

func TestHubStopClean(t *testing.T) {
	h := server.NewNotifyHubWithInterval(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.Start(ctx)

	h.Stop()

	// Give goroutine time to exit
	time.Sleep(20 * time.Millisecond)

	// Should not panic or deadlock
	h.NotifyDashboard()
}

func TestHubStopIdempotent(t *testing.T) {
	h := server.NewNotifyHub()
	// Calling Stop multiple times must not panic.
	h.Stop()
	h.Stop()
}
