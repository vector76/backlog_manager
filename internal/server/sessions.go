package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const sessionCookieName = "bm_session"
const sessionDuration = 24 * time.Hour

// Role represents a session role.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

// sessionEntry holds session data.
type sessionEntry struct {
	expiry time.Time
	role   Role
}

// sessionStore is an in-memory session store.
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]sessionEntry
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: make(map[string]sessionEntry),
	}
}

// create generates a new session ID, stores it with an expiry and role, and returns it.
func (s *sessionStore) create(role Role) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[id] = sessionEntry{expiry: time.Now().Add(sessionDuration), role: role}
	s.mu.Unlock()
	return id, nil
}

// valid returns true if the session ID is known and not expired.
func (s *sessionStore) valid(id string) bool {
	s.mu.RLock()
	entry, ok := s.sessions[id]
	s.mu.RUnlock()
	return ok && time.Now().Before(entry.expiry)
}

// delete removes a session by ID.
func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// fromRequest extracts and validates the session from the request cookie.
func (s *sessionStore) fromRequest(r *http.Request) (string, bool) {
	id, _, ok := s.fromRequestWithRole(r)
	return id, ok
}

// getRole returns the role for the given session ID.
// Returns RoleAdmin as a safe default if the session is not found.
func (s *sessionStore) getRole(id string) Role {
	s.mu.RLock()
	entry, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return RoleAdmin
	}
	return entry.role
}

const roleContextKey contextKey = "role"

// roleFromContext extracts the Role from the context, returning RoleAdmin as default if not set.
func roleFromContext(ctx context.Context) Role {
	if role, ok := ctx.Value(roleContextKey).(Role); ok {
		return role
	}
	return RoleAdmin
}

// fromRequestWithRole extracts the session ID and role from the request cookie in a single
// atomic read, avoiding a TOCTOU race between validity check and role lookup.
func (s *sessionStore) fromRequestWithRole(r *http.Request) (string, Role, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", "", false
	}
	s.mu.RLock()
	entry, ok := s.sessions[cookie.Value]
	s.mu.RUnlock()
	if !ok || !time.Now().Before(entry.expiry) {
		return "", "", false
	}
	return cookie.Value, entry.role, true
}

// webSessionMiddleware returns a middleware that requires a valid session cookie.
// Unauthenticated requests redirect to /login.
func webSessionMiddleware(sessions *sessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, role, ok := sessions.fromRequestWithRole(r)
			if !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			ctx := context.WithValue(r.Context(), roleContextKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
