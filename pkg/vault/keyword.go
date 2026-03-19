package vault

import (
	"strings"

	"context"
	"cube-adk/pkg/core"
)

// KeywordRetriever matches entries by substring containment.
type KeywordRetriever struct{}

func NewKeywordRetriever() *KeywordRetriever { return &KeywordRetriever{} }

func (r *KeywordRetriever) Retrieve(_ context.Context, entries []core.Entry, query string, limit int) ([]core.Fragment, error) {
	q := strings.ToLower(query)
	var results []core.Fragment
	for _, e := range entries {
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
