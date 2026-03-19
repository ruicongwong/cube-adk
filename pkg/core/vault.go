package core

import "context"

// Vault is the unified memory system interface.
type Vault interface {
	Append(ctx context.Context, entry Entry) error
	Recall(ctx context.Context, query string, limit int) ([]Fragment, error)
	Forget(ctx context.Context, filter Filter) error
}

// Embedder converts text into vector embeddings.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

// Retriever selects relevant fragments from a set of entries.
type Retriever interface {
	Retrieve(ctx context.Context, entries []Entry, query string, limit int) ([]Fragment, error)
}
