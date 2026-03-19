package core

import "context"

// Bus provides pub/sub messaging for loosely coupled agent communication.
type Bus interface {
	Publish(ctx context.Context, topic string, sig Signal) error
	Subscribe(ctx context.Context, topic string) (<-chan Signal, error)
	Close() error
}
