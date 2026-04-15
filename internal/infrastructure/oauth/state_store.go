package oauth

import (
	"context"
	"sync"
	"time"
)

// State represents an OAuth state entry.
type State struct {
	Provider    string
	RedirectURL string
	ExpiresAt   time.Time
}

// StateStore defines the interface for OAuth state storage.
type StateStore interface {
	// Add stores a new state with its provider and redirect URL.
	Add(ctx context.Context, state, provider, redirectURL string) error
	// Consume returns the state details when valid and removes it from the store.
	// Returns false when the state is not found or has expired.
	Consume(ctx context.Context, state string) (State, bool, error)
}

type inMemoryStateStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	values map[string]State
}

// NewInMemoryStateStore creates an in-memory OAuth state store.
func NewInMemoryStateStore(ttl time.Duration) StateStore {
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &inMemoryStateStore{
		ttl:    ttl,
		values: make(map[string]State),
	}
}

// Add stores a new state with its provider and redirect URL.
func (s *inMemoryStateStore) Add(_ context.Context, state, provider, redirectURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked()
	s.values[state] = State{
		Provider:    provider,
		RedirectURL: redirectURL,
		ExpiresAt:   time.Now().Add(s.ttl),
	}
	return nil
}

// Consume returns the state details when valid and removes it from the store.
func (s *inMemoryStateStore) Consume(_ context.Context, state string) (State, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked()
	value, ok := s.values[state]
	if !ok {
		return State{}, false, nil
	}
	delete(s.values, state)

	if time.Now().After(value.ExpiresAt) {
		return State{}, false, nil
	}

	return value, true, nil
}

func (s *inMemoryStateStore) cleanupLocked() {
	now := time.Now()
	for key, value := range s.values {
		if now.After(value.ExpiresAt) {
			delete(s.values, key)
		}
	}
}
