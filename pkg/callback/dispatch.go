package callback

import "context"

// OnStart dispatches the OnStart event to all handlers in the context.
func OnStart(ctx context.Context, info RunInfo, input any) context.Context {
	guard := GuardFromContext(ctx)
	if !guard.Active() {
		return ctx
	}
	for _, h := range guard.Handlers() {
		ctx = h.OnStart(ctx, info, input)
	}
	return ctx
}

// OnEnd dispatches the OnEnd event to all handlers in the context.
func OnEnd(ctx context.Context, info RunInfo, output any) context.Context {
	guard := GuardFromContext(ctx)
	if !guard.Active() {
		return ctx
	}
	for _, h := range guard.Handlers() {
		ctx = h.OnEnd(ctx, info, output)
	}
	return ctx
}

// OnError dispatches the OnError event to all handlers in the context.
func OnError(ctx context.Context, info RunInfo, err error) context.Context {
	guard := GuardFromContext(ctx)
	if !guard.Active() {
		return ctx
	}
	for _, h := range guard.Handlers() {
		ctx = h.OnError(ctx, info, err)
	}
	return ctx
}
