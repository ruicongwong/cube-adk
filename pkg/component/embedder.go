package component

import "context"

// Embedder converts text into vector representations.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}
