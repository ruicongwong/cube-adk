package trace

import (
	"context"

	"cube-adk/pkg/core"
)

// WrapBrain returns a Brain that creates trace spans around Think calls.
func WrapBrain(b core.Brain, t core.Tracer) core.Brain {
	if t == nil {
		return b
	}
	return &tracedBrain{inner: b, tracer: t}
}

type tracedBrain struct {
	inner  core.Brain
	tracer core.Tracer
}

func (tb *tracedBrain) Think(ctx context.Context, dialogue []core.Dialogue, tools []core.Tool) (*core.Dialogue, error) {
	ctx, span := tb.tracer.Start(ctx, "brain.think", core.SpanBrain)
	span.SetAttr("dialogue.len", len(dialogue))
	span.SetAttr("tools.count", len(tools))
	resp, err := tb.inner.Think(ctx, dialogue, tools)
	if resp != nil {
		if resp.Usage != nil {
			span.SetAttr("llm.prompt_tokens", resp.Usage.PromptTokens)
			span.SetAttr("llm.completion_tokens", resp.Usage.CompletionTokens)
			span.SetAttr("llm.total_tokens", resp.Usage.TotalTokens)
		}
		if resp.TTFT > 0 {
			span.SetAttr("llm.ttft_ms", resp.TTFT.Milliseconds())
		}
	}
	span.End(err)
	return resp, err
}

// WrapTool returns a Tool that creates trace spans around Perform calls.
// If the inner tool implements ArtifactTool, the wrapper preserves that.
func WrapTool(tool core.Tool, t core.Tracer) core.Tool {
	if t == nil {
		return tool
	}
	tt := &tracedTool{inner: tool, tracer: t}
	if at, ok := tool.(core.ArtifactTool); ok {
		return &tracedArtifactTool{tracedTool: tt, inner: at}
	}
	return tt
}

// WrapTools wraps a slice of tools with tracing.
func WrapTools(tools []core.Tool, t core.Tracer) []core.Tool {
	if t == nil {
		return tools
	}
	out := make([]core.Tool, len(tools))
	for i, tool := range tools {
		out[i] = WrapTool(tool, t)
	}
	return out
}

type tracedTool struct {
	inner  core.Tool
	tracer core.Tracer
}

func (tt *tracedTool) Identity() string       { return tt.inner.Identity() }
func (tt *tracedTool) Brief() string          { return tt.inner.Brief() }
func (tt *tracedTool) Schema() map[string]any { return tt.inner.Schema() }

func (tt *tracedTool) Perform(ctx context.Context, input string) (string, error) {
	ctx, span := tt.tracer.Start(ctx, "tool."+tt.inner.Identity(), core.SpanTool)
	span.SetAttr("input", input)
	out, err := tt.inner.Perform(ctx, input)
	span.SetAttr("output.len", len(out))
	span.End(err)
	return out, err
}

// tracedArtifactTool preserves ArtifactTool interface through tracing.
type tracedArtifactTool struct {
	*tracedTool
	inner core.ArtifactTool
}

func (t *tracedArtifactTool) Artifacts() []core.ArtifactDetail {
	return t.inner.Artifacts()
}
