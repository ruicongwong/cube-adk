package engine

import (
	"context"

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

func (c *ChainAgent) Execute(ctx context.Context, state *core.State) (*protocol.StreamReader[core.Signal], error) {
	r, w := protocol.Pipe[core.Signal](16)
	go c.run(ctx, state, w)
	return r, nil
}

func (c *ChainAgent) run(ctx context.Context, sess *core.State, w *protocol.StreamWriter[core.Signal]) {
	defer w.Finish(nil)

	ctx, hooks := beginHooks(ctx, c.Tracer, c.Name, "agent.chain."+c.Name, core.SpanAgent, sess)
	defer hooks.End(ctx, nil)

	for _, agent := range c.Agents {
		if ctx.Err() != nil {
			return
		}

		sub, err := agent.Execute(ctx, sess)
		if err != nil {
			_ = w.Send(core.Signal{Kind: core.SignalFault, Source: c.Name, Text: err.Error()})
			return
		}

		var lastReply string
		for {
			s, err := sub.Recv()
			if err != nil {
				break
			}
			_ = w.Send(s)
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
