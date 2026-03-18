package core

import "context"

// SpanKind categorizes a trace span.
type SpanKind int

const (
	SpanAgent SpanKind = iota
	SpanBrain
	SpanTool
)

// Span represents a unit of work in a trace.
type Span interface {
	ID() string
	End(err error)
	SetAttr(key string, val any)
	Child(name string, kind SpanKind) Span
}

// Tracer creates and manages trace spans.
type Tracer interface {
	Start(ctx context.Context, name string, kind SpanKind) (context.Context, Span)
}

type spanCtxKey struct{}

// CtxWithSpan returns a new context carrying the given span.
func CtxWithSpan(ctx context.Context, s Span) context.Context {
	return context.WithValue(ctx, spanCtxKey{}, s)
}

// SpanFromCtx extracts the current span from context. Returns nil if none.
func SpanFromCtx(ctx context.Context) Span {
	s, _ := ctx.Value(spanCtxKey{}).(Span)
	return s
}
