package core

import "context"

// Agent is the fundamental execution unit.
type Agent interface {
	Identity() string
	Execute(ctx context.Context, sess *Session) (<-chan Signal, error)
}
