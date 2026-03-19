package component

import (
	"context"

	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// Model generates responses from a sequence of messages.
// Tool binding is handled at the Agent layer via option.WithToolSpecs.
type Model interface {
	Generate(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.Message, error)
	Stream(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.StreamReader[*protocol.Message], error)
}
