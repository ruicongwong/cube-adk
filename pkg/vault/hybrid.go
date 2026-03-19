package vault

import (
	"context"
	"sort"

	"cube-adk/pkg/core"
)

// HybridRetriever combines multiple retrievers using Reciprocal Rank Fusion.
type HybridRetriever struct {
	retrievers []core.Retriever
	k          float64 // RRF constant, default 60
}

func NewHybridRetriever(retrievers ...core.Retriever) *HybridRetriever {
	return &HybridRetriever{retrievers: retrievers, k: 60}
}

func (h *HybridRetriever) Retrieve(ctx context.Context, entries []core.Entry, query string, limit int) ([]core.Fragment, error) {
	scores := make(map[string]*rrfItem)

	for _, r := range h.retrievers {
		frags, err := r.Retrieve(ctx, entries, query, limit*2)
		if err != nil {
			return nil, err
		}
		for rank, f := range frags {
			key := f.Content
			item, ok := scores[key]
			if !ok {
				item = &rrfItem{fragment: f}
				scores[key] = item
			}
			item.score += 1.0 / (h.k + float64(rank+1))
		}
	}

	items := make([]rrfItem, 0, len(scores))
	for _, it := range scores {
		items = append(items, *it)
	}
	sort.Slice(items, func(a, b int) bool { return items[a].score > items[b].score })

	if len(items) > limit {
		items = items[:limit]
	}

	results := make([]core.Fragment, len(items))
	for i, it := range items {
		it.fragment.Score = it.score
		results[i] = it.fragment
	}
	return results, nil
}

type rrfItem struct {
	fragment core.Fragment
	score    float64
}
