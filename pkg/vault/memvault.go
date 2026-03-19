package vault

import (
	"context"
	"sync"

	"cube-adk/pkg/core"
)

// MemVaultOption configures a MemVault.
type MemVaultOption func(*MemVault)

// WithRetriever sets the retrieval strategy. Default is KeywordRetriever.
func WithRetriever(r core.Retriever) MemVaultOption {
	return func(v *MemVault) { v.retriever = r }
}

// MemVault is an in-memory Vault implementation for development and testing.
type MemVault struct {
	entries   []core.Entry
	retriever core.Retriever
	mu        sync.RWMutex
}

func NewMemVault(opts ...MemVaultOption) *MemVault {
	v := &MemVault{retriever: NewKeywordRetriever()}
	for _, o := range opts {
		o(v)
	}
	return v
}

func (v *MemVault) Append(_ context.Context, entry core.Entry) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.entries = append(v.entries, entry)
	return nil
}

func (v *MemVault) Recall(ctx context.Context, query string, limit int) ([]core.Fragment, error) {
	v.mu.RLock()
	entries := make([]core.Entry, len(v.entries))
	copy(entries, v.entries)
	v.mu.RUnlock()

	return v.retriever.Retrieve(ctx, entries, query, limit)
}

func (v *MemVault) Forget(_ context.Context, filter core.Filter) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	filtered := v.entries[:0]
	for _, e := range v.entries {
		if matchFilter(e, filter) {
			continue
		}
		filtered = append(filtered, e)
	}
	v.entries = filtered
	return nil
}

func matchFilter(e core.Entry, f core.Filter) bool {
	if f.Scope != nil && e.Scope != *f.Scope {
		return false
	}
	if f.Tag != "" && e.Tag != f.Tag {
		return false
	}
	return true
}
