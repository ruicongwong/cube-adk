package core

import (
	"cube-adk/pkg/protocol"
	"sync"
)

// Session holds the message history and state for an agent execution.
type Session struct {
	ID      string
	history []*protocol.Message
	state   map[string]any
	vault   Vault
	shelf   Shelf
	mu      sync.RWMutex
}

// SessionOption configures a Session.
type SessionOption func(*Session)

// WithVault attaches a Vault to the session.
func WithVault(v Vault) SessionOption {
	return func(s *Session) { s.vault = v }
}

// WithShelf attaches a Shelf to the session.
func WithShelf(sh Shelf) SessionOption {
	return func(s *Session) { s.shelf = sh }
}

// NewSession creates a new Session with the given options.
func NewSession(id string, opts ...SessionOption) *Session {
	s := &Session{
		ID:    id,
		state: make(map[string]any),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Append adds a message to the history.
func (s *Session) Append(m *protocol.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, m)
}

// History returns a copy of the message history.
func (s *Session) History() []*protocol.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*protocol.Message, len(s.history))
	copy(out, s.history)
	return out
}

// Vault returns the associated vault, may be nil.
func (s *Session) Vault() Vault { return s.vault }

// Shelf returns the associated shelf, may be nil.
func (s *Session) Shelf() Shelf { return s.shelf }

// Set stores a key-value pair in session state.
func (s *Session) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[key] = val
}

// Get retrieves a value from session state.
func (s *Session) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.state[key]
	return v, ok
}
