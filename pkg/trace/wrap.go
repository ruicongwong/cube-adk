package trace

import (
	"context"

	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// WrapModel returns a Model that creates trace spans around Generate calls.
func WrapModel(m component.Model, t core.Tracer) component.Model {
	if t == nil {
		return m
	}
	return &tracedModel{inner: m, tracer: t}
}

type tracedModel struct {
	inner  component.Model
	tracer core.Tracer
}

func (tm *tracedModel) Generate(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.Message, error) {
	ctx, span := tm.tracer.Start(ctx, "model.generate", core.SpanBrain)
	span.SetAttr("messages.len", len(msgs))
	resp, err := tm.inner.Generate(ctx, msgs, opts...)
	if resp != nil && resp.TokenUsage != nil {
		span.SetAttr("llm.prompt_tokens", resp.TokenUsage.PromptTokens)
		span.SetAttr("llm.completion_tokens", resp.TokenUsage.CompletionTokens)
		span.SetAttr("llm.total_tokens", resp.TokenUsage.TotalTokens)
	}
	span.End(err)
	return resp, err
}

func (tm *tracedModel) Stream(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.StreamReader[*protocol.Message], error) {
	_, span := tm.tracer.Start(ctx, "model.stream", core.SpanBrain)
	span.SetAttr("messages.len", len(msgs))
	r, err := tm.inner.Stream(ctx, msgs, opts...)
	if err != nil {
		span.End(err)
		return nil, err
	}
	// Note: span will not be ended until stream is consumed
	return r, nil
}

// WrapTool returns a Tool that creates trace spans around Run calls.
func WrapTool(t component.Tool, tr core.Tracer) component.Tool {
	if tr == nil {
		return t
	}
	return &tracedTool{inner: t, tracer: tr}
}

// WrapTools wraps a slice of tools with tracing.
func WrapTools(tools []component.Tool, t core.Tracer) []component.Tool {
	if t == nil {
		return tools
	}
	out := make([]component.Tool, len(tools))
	for i, tl := range tools {
		out[i] = WrapTool(tl, t)
	}
	return out
}

type tracedTool struct {
	inner  component.Tool
	tracer core.Tracer
}

func (tt *tracedTool) Identity() string        { return tt.inner.Identity() }
func (tt *tracedTool) Brief() string           { return tt.inner.Brief() }
func (tt *tracedTool) Spec() protocol.ToolSpec { return tt.inner.Spec() }

func (tt *tracedTool) Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error) {
	ctx, span := tt.tracer.Start(ctx, "tool."+tt.inner.Identity(), core.SpanTool)
	span.SetAttr("tool.call_id", call.ID)
	span.SetAttr("tool.name", call.Name)
	result, err := tt.inner.Run(ctx, call, opts...)
	span.SetAttr("tool.failed", result.Failed)
	span.End(err)
	return result, err
}
