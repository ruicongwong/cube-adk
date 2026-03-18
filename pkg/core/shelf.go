package core

import "context"

// Shelf stores and retrieves artifacts produced by agents.
type Shelf interface {
	// Store persists an artifact. Overwrites if same ID exists.
	Store(ctx context.Context, artifact ArtifactDetail) error
	// Fetch retrieves an artifact by ID.
	Fetch(ctx context.Context, id string) (*ArtifactDetail, error)
	// List returns artifact metadata (Data=nil) matching the optional MIME filter.
	List(ctx context.Context, mime string, limit int) ([]ArtifactDetail, error)
	// Discard removes an artifact by ID.
	Discard(ctx context.Context, id string) error
}
