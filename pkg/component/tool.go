package component

import (
	"context"

	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// Tool executes a specific capability and returns structured results.
type Tool interface {
	Identity() string
	Brief() string
	Spec() protocol.ToolSpec
	Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error)
}
