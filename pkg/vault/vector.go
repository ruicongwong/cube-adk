package vault

import (
	"context"
	"crypto/sha256"
	"math"
	"sort"
	"sync"

	"cube-adk/pkg/core"
)

// VectorRetriever uses an Embedder to perform cosine-similarity based retrieval.
type VectorRetriever struct {
	embedder core.Embedder
	cache    map[string][]float64
	mu       sync.RWMutex
}

func NewVectorRetriever(e core.Embedder) *VectorRetriever {
	return &VectorRetriever{embedder: e, cache: make(map[string][]float64)}
}

func (r *VectorRetriever) Retrieve(ctx context.Context, entries []core.Entry, query string, limit int) ([]core.Fragment, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	// Embed query
	qvecs, err := r.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	qvec := qvecs[0]

	// Embed entries that are not yet cached
	var toEmbed []string
	var toEmbedIdx []int
	r.mu.RLock()
	for i, e := range entries {
		key := contentKey(e.Content)
		if _, ok := r.cache[key]; !ok {
			toEmbed = append(toEmbed, e.Content)
			toEmbedIdx = append(toEmbedIdx, i)
		}
	}
	r.mu.RUnlock()

	if len(toEmbed) > 0 {
		vecs, err := r.embedder.Embed(ctx, toEmbed)
		if err != nil {
			return nil, err
		}
		r.mu.Lock()
		for j, idx := range toEmbedIdx {
			r.cache[contentKey(entries[idx].Content)] = vecs[j]
		}
		r.mu.Unlock()
	}

	// Score all entries
	type scored struct {
		idx   int
		score float64
	}
	var items []scored
	r.mu.RLock()
	for i, e := range entries {
		key := contentKey(e.Content)
		if vec, ok := r.cache[key]; ok {
			items = append(items, scored{idx: i, score: cosineSim(qvec, vec)})
		}
	}
	r.mu.RUnlock()

	sort.Slice(items, func(a, b int) bool { return items[a].score > items[b].score })

	if len(items) > limit {
		items = items[:limit]
	}

	results := make([]core.Fragment, len(items))
	for i, it := range items {
		e := entries[it.idx]
		results[i] = core.Fragment{Content: e.Content, Score: it.score, Source: e.Tag}
	}
	return results, nil
}

func contentKey(s string) string {
	h := sha256.Sum256([]byte(s))
	return string(h[:])
}

func cosineSim(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	denom := math.Sqrt(na) * math.Sqrt(nb)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
