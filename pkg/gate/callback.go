package gate

import (
	"context"

	"cube-adk/pkg/core"
)

// CallbackGate delegates approval to a function.
type CallbackGate struct {
	Fn func(core.Checkpoint) (core.Review, error)
}

func NewCallbackGate(fn func(core.Checkpoint) (core.Review, error)) *CallbackGate {
	return &CallbackGate{Fn: fn}
}

func (g *CallbackGate) Check(_ context.Context, cp core.Checkpoint) (core.Review, error) {
	return g.Fn(cp)
}

// ChannelGate sends checkpoints to a channel and receives reviews from another.
// Useful for web UI / Slack bot integration.
type ChannelGate struct {
	Out chan<- core.Checkpoint
	In  <-chan core.Review
}

func NewChannelGate(out chan<- core.Checkpoint, in <-chan core.Review) *ChannelGate {
	return &ChannelGate{Out: out, In: in}
}

func (g *ChannelGate) Check(ctx context.Context, cp core.Checkpoint) (core.Review, error) {
	select {
	case g.Out <- cp:
	case <-ctx.Done():
		return core.Review{Verdict: core.Reject, Reason: "context cancelled"}, ctx.Err()
	}
	select {
	case review := <-g.In:
		return review, nil
	case <-ctx.Done():
		return core.Review{Verdict: core.Reject, Reason: "context cancelled"}, ctx.Err()
	}
}
