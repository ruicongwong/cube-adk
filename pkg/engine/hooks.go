package engine

import (
	"context"

	"cube-adk/pkg/callback"
	"cube-adk/pkg/core"
)

// hookScope bundles callback and trace lifecycle into a single defer-friendly handle.
type hookScope struct {
	info callback.RunInfo
	span core.Span
}

// beginHooks fires OnStart and opens a trace span (if tracer is non-nil).
func beginHooks(ctx context.Context, tracer core.Tracer, name, spanName string, kind core.SpanKind, input any) (context.Context, *hookScope) {
	info := callback.RunInfo{Name: name, Kind: "agent"}
	ctx = callback.OnStart(ctx, info, input)
	var span core.Span
	if tracer != nil {
		ctx, span = tracer.Start(ctx, spanName, kind)
	}
	return ctx, &hookScope{info: info, span: span}
}

// End closes the trace span and fires OnEnd.
func (h *hookScope) End(ctx context.Context, output any) {
	if h.span != nil {
		h.span.End(nil)
	}
	callback.OnEnd(ctx, h.info, output)
}

// Error fires OnError and closes the trace span with the error.
func (h *hookScope) Error(ctx context.Context, err error) {
	callback.OnError(ctx, h.info, err)
	if h.span != nil {
		h.span.End(err)
	}
}
