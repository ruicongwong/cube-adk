package bus

import (
	"context"
	"sync"

	"cube-adk/pkg/core"
)

// MemBus is an in-memory pub/sub Bus for local agent communication.
type MemBus struct {
	mu     sync.RWMutex
	subs   map[string][]chan core.Signal
	closed bool
}

func NewMemBus() *MemBus {
	return &MemBus{
		subs: make(map[string][]chan core.Signal),
	}
}

func (b *MemBus) Publish(_ context.Context, topic string, sig core.Signal) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil
	}

	for _, ch := range b.subs[topic] {
		select {
		case ch <- sig:
		default:
			// drop if subscriber is slow — non-blocking
		}
	}
	return nil
}

func (b *MemBus) Subscribe(_ context.Context, topic string) (<-chan core.Signal, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan core.Signal, 64)
	b.subs[topic] = append(b.subs[topic], ch)
	return ch, nil
}

func (b *MemBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for topic, chs := range b.subs {
		for _, ch := range chs {
			close(ch)
		}
		delete(b.subs, topic)
	}
	return nil
}
