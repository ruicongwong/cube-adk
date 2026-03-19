package option

import "cube-adk/pkg/protocol"

// ModelOpts holds common options for model generation.
type ModelOpts struct {
	Temperature *float64
	MaxTokens   *int
	TopP        *float64
	StopWords   []string
	ToolSpecs   []protocol.ToolSpec
}

// ModelOption is a functional option for model generation.
type ModelOption = Option[ModelOpts]

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) ModelOption {
	return NewOption[ModelOpts](func(o *ModelOpts) { o.Temperature = &t })
}

// WithMaxTokens sets the maximum number of tokens to generate.
func WithMaxTokens(n int) ModelOption {
	return NewOption[ModelOpts](func(o *ModelOpts) { o.MaxTokens = &n })
}

// WithTopP sets the nucleus sampling parameter.
func WithTopP(p float64) ModelOption {
	return NewOption[ModelOpts](func(o *ModelOpts) { o.TopP = &p })
}

// WithStopWords sets the stop sequences.
func WithStopWords(words ...string) ModelOption {
	return NewOption[ModelOpts](func(o *ModelOpts) { o.StopWords = words })
}

// WithToolSpecs sets the tool specifications for the model to use.
func WithToolSpecs(specs ...protocol.ToolSpec) ModelOption {
	return NewOption[ModelOpts](func(o *ModelOpts) { o.ToolSpecs = specs })
}
