package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const sessionCookieName = "bm_session"
const sessionDuration = 24 * time.Hour

// sessionStore is an in-memory session store.
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: make(map[string]time.Time),
	}
}

// create generates a new session ID, stores it with an expiry, and returns it.
func (s *sessionStore) create() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[id] = time.Now().Add(sessionDuration)
	s.mu.Unlock()
	return id, nil
}

// valid returns true if the session ID is known and not expired.
func (s *sessionStore) valid(id string) bool {
	s.mu.RLock()
	expiry, ok := s.sessions[id]
	s.mu.RUnlock()
	return ok && time.Now().Before(expiry)
}

// delete removes a session by ID.
func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// fromRequest extracts and validates the session from the request cookie.
func (s *sessionStore) fromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	if !s.valid(cookie.Value) {
		return "", false
	}
	return cookie.Value, true
}

// webSessionMiddleware returns a middleware that requires a valid session cookie.
// Unauthenticated requests redirect to /login.
func webSessionMiddleware(sessions *sessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := sessions.fromRequest(r); !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
