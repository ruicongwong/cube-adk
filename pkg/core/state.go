package core

import (
	"cube-adk/pkg/protocol"
	"sync"
)

// State holds the message history and state for an agent execution.
type State struct {
	ID      string
	history []*protocol.Message
	state   map[string]any
	vault   Vault
	shelf   Shelf
	mu      sync.RWMutex
}

// StateOption configures a State.
type StateOption func(*State)

// WithVault attaches a Vault to the session.
func WithVault(v Vault) StateOption {
	return func(s *State) { s.vault = v }
}

// WithShelf attaches a Shelf to the session.
func WithShelf(sh Shelf) StateOption {
	return func(s *State) { s.shelf = sh }
}

// NewState creates a new State with the given options.
func NewState(id string, opts ...StateOption) *State {
	s := &State{
		ID:    id,
		state: make(map[string]any),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Append adds a message to the history.
func (s *State) Append(m *protocol.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, m)
}

// History returns a copy of the message history.
func (s *State) History() []*protocol.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*protocol.Message, len(s.history))
	copy(out, s.history)
	return out
}

// Vault returns the associated vault, may be nil.
func (s *State) Vault() Vault { return s.vault }

// Shelf returns the associated shelf, may be nil.
func (s *State) Shelf() Shelf { return s.shelf }

// Set stores a key-value pair in session state.
func (s *State) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[key] = val
}

// Get retrieves a value from session state.
func (s *State) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.state[key]
	return v, ok
}
