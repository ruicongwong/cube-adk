package option

import "time"

// ToolOpts holds common options for tool execution.
type ToolOpts struct {
	Timeout    time.Duration
	RetryCount int
}

// ToolOption is a functional option for tool execution.
type ToolOption = Option[ToolOpts]

// WithTimeout sets the tool execution timeout.
func WithTimeout(d time.Duration) ToolOption {
	return NewOption[ToolOpts](func(o *ToolOpts) { o.Timeout = d })
}

// WithRetryCount sets the number of retries on failure.
func WithRetryCount(n int) ToolOption {
	return NewOption[ToolOpts](func(o *ToolOpts) { o.RetryCount = n })
}
