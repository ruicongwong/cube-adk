package core

import "context"

// Brain abstracts an LLM backend.
type Brain interface {
	Think(ctx context.Context, dialogue []Dialogue, tools []Tool) (*Dialogue, error)
}
