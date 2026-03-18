package core

import "context"

// Bus provides pub/sub messaging for loosely coupled agent communication.
// Agents publish signals to topics and subscribe to receive them,
// enabling Dapr-style decoupled orchestration.
type Bus interface {
	// Publish sends a signal to a named topic.
	Publish(ctx context.Context, topic string, sig Signal) error
	// Subscribe returns a channel that receives signals from a named topic.
	Subscribe(ctx context.Context, topic string) (<-chan Signal, error)
	// Close shuts down the bus and all subscriptions.
	Close() error
}
