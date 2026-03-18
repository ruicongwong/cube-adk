package shelf

import (
	"context"
	"fmt"
	"sync"

	"cube-adk/pkg/core"
)

// MemShelf is an in-memory Shelf implementation for development and testing.
type MemShelf struct {
	items map[string]core.ArtifactDetail
	mu    sync.RWMutex
}

func NewMemShelf() *MemShelf {
	return &MemShelf{items: make(map[string]core.ArtifactDetail)}
}

func (s *MemShelf) Store(_ context.Context, a core.ArtifactDetail) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[a.ID] = a
	return nil
}

func (s *MemShelf) Fetch(_ context.Context, id string) (*core.ArtifactDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("shelf: artifact %q not found", id)
	}
	return &a, nil
}

func (s *MemShelf) List(_ context.Context, mime string, limit int) ([]core.ArtifactDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []core.ArtifactDetail
	for _, a := range s.items {
		if mime != "" && a.MIME != mime {
			continue
		}
		out = append(out, core.ArtifactDetail{ID: a.ID, Name: a.Name, MIME: a.MIME, Meta: a.Meta})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *MemShelf) Discard(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	return nil
}
