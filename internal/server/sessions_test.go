package server

import (
	"testing"
	"time"
)

func TestCreateAdminSession(t *testing.T) {
	s := newSessionStore()
	id, err := s.create(RoleAdmin)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if !s.valid(id) {
		t.Error("expected session to be valid")
	}
	if role := s.getRole(id); role != RoleAdmin {
		t.Errorf("expected RoleAdmin, got %q", role)
	}
}

func TestCreateViewerSession(t *testing.T) {
	s := newSessionStore()
	id, err := s.create(RoleViewer)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if !s.valid(id) {
		t.Error("expected session to be valid")
	}
	if role := s.getRole(id); role != RoleViewer {
		t.Errorf("expected RoleViewer, got %q", role)
	}
}

func TestValidUnknownSession(t *testing.T) {
	s := newSessionStore()
	if s.valid("nonexistent") {
		t.Error("expected valid to return false for unknown session")
	}
}

func TestDeleteSession(t *testing.T) {
	s := newSessionStore()
	id, err := s.create(RoleAdmin)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	s.delete(id)
	if s.valid(id) {
		t.Error("expected session to be invalid after delete")
	}
}

func TestExpiredSession(t *testing.T) {
	s := newSessionStore()
	id := "expired-session-id"
	s.sessions[id] = sessionEntry{expiry: time.Now().Add(-time.Minute), role: RoleViewer}
	if s.valid(id) {
		t.Error("expected expired session to be invalid")
	}
}
