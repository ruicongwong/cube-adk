package callback

import "context"

// Handler receives lifecycle events during component execution.
type Handler interface {
	OnStart(ctx context.Context, info RunInfo, input any) context.Context
	OnEnd(ctx context.Context, info RunInfo, output any) context.Context
	OnError(ctx context.Context, info RunInfo, err error) context.Context
}

// handlerFn is a concrete Handler built by HandlerBuilder.
type handlerFn struct {
	startFn func(context.Context, RunInfo, any) context.Context
	endFn   func(context.Context, RunInfo, any) context.Context
	errorFn func(context.Context, RunInfo, error) context.Context
}

func (h *handlerFn) OnStart(ctx context.Context, info RunInfo, input any) context.Context {
	if h.startFn != nil {
		return h.startFn(ctx, info, input)
	}
	return ctx
}

func (h *handlerFn) OnEnd(ctx context.Context, info RunInfo, output any) context.Context {
	if h.endFn != nil {
		return h.endFn(ctx, info, output)
	}
	return ctx
}

func (h *handlerFn) OnError(ctx context.Context, info RunInfo, err error) context.Context {
	if h.errorFn != nil {
		return h.errorFn(ctx, info, err)
	}
	return ctx
}

// HandlerBuilder constructs a Handler using a fluent API.
type HandlerBuilder struct {
	h handlerFn
}

// NewHandler starts building a new Handler.
func NewHandler() *HandlerBuilder {
	return &HandlerBuilder{}
}

// Start sets the OnStart callback.
func (b *HandlerBuilder) Start(fn func(context.Context, RunInfo, any) context.Context) *HandlerBuilder {
	b.h.startFn = fn
	return b
}

// End sets the OnEnd callback.
func (b *HandlerBuilder) End(fn func(context.Context, RunInfo, any) context.Context) *HandlerBuilder {
	b.h.endFn = fn
	return b
}

// Error sets the OnError callback.
func (b *HandlerBuilder) Error(fn func(context.Context, RunInfo, error) context.Context) *HandlerBuilder {
	b.h.errorFn = fn
	return b
}

// Build returns the constructed Handler.
func (b *HandlerBuilder) Build() Handler {
	cp := b.h
	return &cp
}
