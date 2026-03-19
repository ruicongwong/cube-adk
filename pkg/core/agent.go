package core

import (
	"context"

	"cube-adk/pkg/protocol"
)

// Agent is the fundamental execution unit.
type Agent interface {
	Identity() string
	Execute(ctx context.Context, state *State) (*protocol.StreamReader[Signal], error)
}
