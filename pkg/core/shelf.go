package core

import "context"

// Shelf stores and retrieves artifacts produced by agents.
type Shelf interface {
	Store(ctx context.Context, artifact ArtifactDetail) error
	Fetch(ctx context.Context, id string) (*ArtifactDetail, error)
	List(ctx context.Context, mime string, limit int) ([]ArtifactDetail, error)
	Discard(ctx context.Context, id string) error
}
