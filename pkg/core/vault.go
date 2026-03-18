package core

import "context"

// Vault is the unified memory system interface.
type Vault interface {
	Append(ctx context.Context, entry Entry) error
	Recall(ctx context.Context, query string, limit int) ([]Fragment, error)
	Forget(ctx context.Context, filter Filter) error
}
