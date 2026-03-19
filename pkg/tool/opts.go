package tool

import (
	"context"

	"cube-adk/pkg/option"
)

// applyToolOpts resolves ToolOpts from options and returns a context with
// timeout (if set), a cleanup function, and the number of attempts.
func applyToolOpts(ctx context.Context, opts ...option.ToolOption) (context.Context, func(), int) {
	var to option.ToolOpts
	option.Apply(&to, opts...)
	cleanup := func() {}
	if to.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, to.Timeout)
		cleanup = cancel
	}
	return ctx, cleanup, max(1, 1+to.RetryCount)
}
