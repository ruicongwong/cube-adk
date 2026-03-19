package component

import (
	"context"

	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// Retriever fetches relevant documents for a given query.
type Retriever interface {
	Retrieve(ctx context.Context, query string, opts ...option.RetrieverOption) ([]*protocol.Document, error)
}
