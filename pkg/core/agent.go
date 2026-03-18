package core

import (
	"context"
	"sync"
)

// Agent is the fundamental execution unit.
type Agent interface {
	Identity() string
	Execute(ctx context.Context, conv *Conversation) (<-chan Signal, error)
}

// Conversation holds the dialogue history and optional vault/shelf for an agent session.
type Conversation struct {
	ID      string
	history []Dialogue
	state   map[string]any
	vault   Vault
	shelf   Shelf
	mu      sync.RWMutex
}

// ConvOption configures a Conversation.
type ConvOption func(*Conversation)

// WithVault attaches a Vault to the conversation.
func WithVault(v Vault) ConvOption {
	return func(c *Conversation) { c.vault = v }
}

// WithShelf attaches a Shelf to the conversation.
func WithShelf(s Shelf) ConvOption {
	return func(c *Conversation) { c.shelf = s }
}

// NewConversation creates a new Conversation with the given options.
func NewConversation(id string, opts ...ConvOption) *Conversation {
	c := &Conversation{
		ID:    id,
		state: make(map[string]any),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Append adds a dialogue entry to the history.
func (c *Conversation) Append(d Dialogue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = append(c.history, d)
}

// History returns a copy of the dialogue history.
func (c *Conversation) History() []Dialogue {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Dialogue, len(c.history))
	copy(out, c.history)
	return out
}

// Vault returns the associated vault, may be nil.
func (c *Conversation) Vault() Vault { return c.vault }

// Shelf returns the associated shelf, may be nil.
func (c *Conversation) Shelf() Shelf { return c.shelf }

// Set stores a key-value pair in conversation state.
func (c *Conversation) Set(key string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state[key] = val
}

// Get retrieves a value from conversation state.
func (c *Conversation) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.state[key]
	return v, ok
}
