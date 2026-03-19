package callback

import "context"

// TimingGuard wraps a set of handlers and provides a fast Active() check
// to skip callback overhead when no handlers are registered.
type TimingGuard struct {
	handlers []Handler
}

// NewTimingGuard creates a guard from the given handlers.
func NewTimingGuard(handlers []Handler) *TimingGuard {
	return &TimingGuard{handlers: handlers}
}

// GuardFromContext creates a TimingGuard from handlers in the context.
func GuardFromContext(ctx context.Context) *TimingGuard {
	return NewTimingGuard(Extract(ctx))
}

// Active returns true if there are any handlers to invoke.
func (g *TimingGuard) Active() bool {
	return len(g.handlers) > 0
}

// Handlers returns the underlying handler slice.
func (g *TimingGuard) Handlers() []Handler {
	return g.handlers
}
