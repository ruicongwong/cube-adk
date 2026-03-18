package engine

import (
	"context"

	"cube-adk/pkg/core"
)

// ChainAgent executes a sequence of agents, sharing the same Conversation.
type ChainAgent struct {
	Name   string
	Agents []core.Agent
	Tracer core.Tracer
}

func (c *ChainAgent) Identity() string { return c.Name }

func (c *ChainAgent) Execute(ctx context.Context, conv *core.Conversation) (<-chan core.Signal, error) {
	ch := make(chan core.Signal, 16)
	go c.run(ctx, conv, ch)
	return ch, nil
}

func (c *ChainAgent) run(ctx context.Context, conv *core.Conversation, ch chan<- core.Signal) {
	defer close(ch)

	var span core.Span
	if c.Tracer != nil {
		ctx, span = c.Tracer.Start(ctx, "agent.chain."+c.Name, core.SpanAgent)
		defer span.End(nil)
	}

	for _, agent := range c.Agents {
		if ctx.Err() != nil {
			return
		}

		sub, err := agent.Execute(ctx, conv)
		if err != nil {
			ch <- core.Signal{Kind: core.SignalFault, Source: c.Name, Text: err.Error()}
			return
		}

		// Forward all signals, capture the last reply
		var lastReply string
		for s := range sub {
			ch <- s
			if s.Kind == core.SignalReply {
				lastReply = s.Text
			}
			// If a sub-agent hands off, stop the chain and propagate
			if s.Kind == core.SignalHandoff {
				return
			}
			if s.Kind == core.SignalFault {
				return
			}
		}

		// Inject the reply as a user message for the next agent in the chain
		if lastReply != "" {
			conv.Append(core.Dialogue{Role: "user", Text: lastReply})
		}
	}
}
