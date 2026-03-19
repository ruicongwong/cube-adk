package engine

import (
	"context"

	"cube-adk/pkg/callback"
	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
	"cube-adk/pkg/protocol"
)

// ChainAgent executes a sequence of agents, sharing the same Session.
type ChainAgent struct {
	Name   string
	Agents []core.Agent
	Tracer core.Tracer
}

func (c *ChainAgent) Identity() string { return c.Name }

func (c *ChainAgent) Execute(ctx context.Context, sess *core.Session) (<-chan core.Signal, error) {
	ch := make(chan core.Signal, 16)
	go c.run(ctx, sess, ch)
	return ch, nil
}

func (c *ChainAgent) run(ctx context.Context, sess *core.Session, ch chan<- core.Signal) {
	defer close(ch)

	info := callback.RunInfo{Name: c.Name, Kind: "agent", Component: component.KindModel}
	ctx = callback.OnStart(ctx, info, sess)
	defer func() { callback.OnEnd(ctx, info, nil) }()

	var span core.Span
	if c.Tracer != nil {
		ctx, span = c.Tracer.Start(ctx, "agent.chain."+c.Name, core.SpanAgent)
		defer span.End(nil)
	}

	for _, agent := range c.Agents {
		if ctx.Err() != nil {
			return
		}

		sub, err := agent.Execute(ctx, sess)
		if err != nil {
			ch <- core.Signal{Kind: core.SignalFault, Source: c.Name, Text: err.Error()}
			return
		}

		var lastReply string
		for s := range sub {
			ch <- s
			if s.Kind == core.SignalReply {
				lastReply = s.Text
			}
			if s.Kind == core.SignalHandoff || s.Kind == core.SignalFault {
				return
			}
		}

		if lastReply != "" {
			sess.Append(protocol.NewTextMessage("user", lastReply))
		}
	}
}
