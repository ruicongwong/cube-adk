package vault

import (
	"context"
	"strings"
	"sync"

	"cube-adk/pkg/core"
)

// MemVault is an in-memory Vault implementation for development and testing.
type MemVault struct {
	entries []core.Entry
	mu      sync.RWMutex
}

func NewMemVault() *MemVault {
	return &MemVault{}
}

func (v *MemVault) Append(_ context.Context, entry core.Entry) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.entries = append(v.entries, entry)
	return nil
}

func (v *MemVault) Recall(_ context.Context, query string, limit int) ([]core.Fragment, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var results []core.Fragment
	q := strings.ToLower(query)
	for _, e := range v.entries {
		if strings.Contains(strings.ToLower(e.Content), q) {
			results = append(results, core.Fragment{
				Content: e.Content,
				Score:   1.0,
				Source:  e.Tag,
			})
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func (v *MemVault) Forget(_ context.Context, filter core.Filter) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	filtered := v.entries[:0]
	for _, e := range v.entries {
		if matchFilter(e, filter) {
			continue // remove
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
