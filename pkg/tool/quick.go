package tool

import (
	"context"

	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// QuickTool wraps a plain function as a component.Tool.
type QuickTool struct {
	Name   string
	Desc   string
	Params map[string]any
	Fn     func(ctx context.Context, args string) (string, error)
	Opts   []option.ToolOption // default options applied on every Run
}

func (t *QuickTool) Identity() string { return t.Name }
func (t *QuickTool) Brief() string    { return t.Desc }

func (t *QuickTool) Spec() protocol.ToolSpec {
	params := t.Params
	if params == nil {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return protocol.ToolSpec{Name: t.Name, Desc: t.Desc, Schema: params}
}

func (t *QuickTool) Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error) {
	all := append(t.Opts, opts...)
	ctx, cleanup, attempt := applyToolOpts(ctx, all...)
	defer cleanup()
	var result protocol.ToolResult
	for i := 0; i < attempt; i++ {
		output, err := t.Fn(ctx, call.Args)
		if err != nil {
			result = protocol.NewErrorResult(call.ID, err)
			continue
		}
		return protocol.NewTextResult(call.ID, output), nil
	}
	return result, nil
}
