package callback

import "context"

type ctxKey struct{}

// Inject adds callback handlers to the context.
func Inject(ctx context.Context, handlers ...Handler) context.Context {
	existing := Extract(ctx)
	all := make([]Handler, 0, len(existing)+len(handlers))
	all = append(all, existing...)
	all = append(all, handlers...)
	return context.WithValue(ctx, ctxKey{}, all)
}

// Extract retrieves all callback handlers from the context.
func Extract(ctx context.Context) []Handler {
	if v, ok := ctx.Value(ctxKey{}).([]Handler); ok {
		return v
	}
	return nil
}

type runInfoKey struct{}

// WithRunInfo attaches RunInfo to the context.
func WithRunInfo(ctx context.Context, info RunInfo) context.Context {
	return context.WithValue(ctx, runInfoKey{}, info)
}

// RunInfoFrom retrieves RunInfo from the context.
func RunInfoFrom(ctx context.Context) (RunInfo, bool) {
	info, ok := ctx.Value(runInfoKey{}).(RunInfo)
	return info, ok
}
