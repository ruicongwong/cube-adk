package trace

import (
	"context"

	"cube-adk/pkg/core"
)

// NopTracer is a zero-cost tracer that does nothing.
var Nop core.Tracer = &nopTracer{}

type nopTracer struct{}

func (t *nopTracer) Start(ctx context.Context, _ string, _ core.SpanKind) (context.Context, core.Span) {
	return ctx, nopSpan{}
}

type nopSpan struct{}

func (nopSpan) ID() string                                { return "" }
func (nopSpan) End(_ error)                               {}
func (nopSpan) SetAttr(_ string, _ any)                   {}
func (nopSpan) Child(_ string, _ core.SpanKind) core.Span { return nopSpan{} }
