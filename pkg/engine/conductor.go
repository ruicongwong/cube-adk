package engine

import (
	"context"
	"fmt"
	"strings"

	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
	"cube-adk/pkg/tool"
)

// ToolInjector is implemented by agents that accept framework-managed tools.
type ToolInjector interface {
	InjectTools(tools ...component.Tool)
}

// Conductor orchestrates multiple agents with handoff support.
type Conductor struct {
	Name       string
	Agents     map[string]core.Agent
	EntryAgent string
	Tracer     core.Tracer
}

func NewConductor(name, entry string, agents ...core.Agent) *Conductor {
	m := make(map[string]core.Agent, len(agents))
	for _, a := range agents {
		m[a.Identity()] = a
	}
	c := &Conductor{Name: name, Agents: m, EntryAgent: entry}
	c.injectHandoffTools()
	return c
}

func (c *Conductor) injectHandoffTools() {
	names := make([]string, 0, len(c.Agents))
	for n := range c.Agents {
		names = append(names, n)
	}

	for id, agent := range c.Agents {
		injector, ok := agent.(ToolInjector)
		if !ok {
			continue
		}

		var peers []string
		for _, n := range names {
			if n != id {
				peers = append(peers, n)
			}
		}
		if len(peers) == 0 {
			continue
		}

		injector.InjectTools(newHandoffTool(peers))
	}
}

func newHandoffTool(peers []string) component.Tool {
	desc := fmt.Sprintf(
		"Transfer the conversation to another agent. Available targets: [%s]. Input: JSON {\"target\": \"agent_name\"}",
		strings.Join(peers, ", "),
	)
	return &tool.QuickTool{
		Name: "handoff",
		Desc: desc,
		Params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"enum":        peers,
					"description": "The name of the agent to hand off to",
				},
			},
			"required": []string{"target"},
		},
		Fn: func(_ context.Context, input string) (string, error) {
			return "handoff initiated", nil
		},
	}
}

func (c *Conductor) Identity() string { return c.Name }

func (c *Conductor) Execute(ctx context.Context, state *core.State) (*protocol.StreamReader[core.Signal], error) {
	r, w := protocol.Pipe[core.Signal](16)
	go c.run(ctx, state, w)
	return r, nil
}

func (c *Conductor) run(ctx context.Context, sess *core.State, w *protocol.StreamWriter[core.Signal]) {
	defer w.Finish(nil)

	ctx, hooks := beginHooks(ctx, c.Tracer, c.Name, "agent.conductor."+c.Name, core.SpanAgent, sess)
	defer hooks.End(ctx, nil)

	current := c.EntryAgent
	visited := make(map[string]int)

	for {
		if ctx.Err() != nil {
			return
		}

		agent, ok := c.Agents[current]
		if !ok {
			_ = w.Send(core.Signal{Kind: core.SignalFault, Source: c.Name,
				Text: fmt.Sprintf("unknown agent: %s", current)})
			return
		}

		visited[current]++
		if visited[current] > 10 {
			_ = w.Send(core.Signal{Kind: core.SignalFault, Source: c.Name,
				Text: fmt.Sprintf("agent %s exceeded max handoff visits", current)})
			return
		}

		sub, err := agent.Execute(ctx, sess)
		if err != nil {
			_ = w.Send(core.Signal{Kind: core.SignalFault, Source: c.Name, Text: err.Error()})
			return
		}

		handoff := ""
		for {
			s, err := sub.Recv()
			if err != nil {
				break
			}
			_ = w.Send(s)
			if s.Kind == core.SignalHandoff {
				handoff = s.Handoff
			}
		}

		if handoff == "" {
			return
		}
		current = handoff
	}
}

// AsTool wraps an Agent as a component.Tool so it can be used by other agents.
func AsTool(agent core.Agent) component.Tool {
	return &agentTool{agent: agent}
}

type agentTool struct {
	agent core.Agent
}

func (t *agentTool) Identity() string { return t.agent.Identity() }
func (t *agentTool) Brief() string    { return "Delegate to agent: " + t.agent.Identity() }

func (t *agentTool) Spec() protocol.ToolSpec {
	return protocol.ToolSpec{
		Name: t.agent.Identity(),
		Desc: t.Brief(),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string", "description": "The task to delegate"},
			},
			"required": []string{"input"},
		},
	}
}

func (t *agentTool) Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error) {
	sess := core.NewState("delegate")
	sess.Append(protocol.NewTextMessage("user", call.Args))

	r, err := t.agent.Execute(ctx, sess)
	if err != nil {
		return protocol.NewErrorResult(call.ID, err), nil
	}

	var reply string
	for {
		sig, err := r.Recv()
		if err != nil {
			break
		}
		if sig.Kind == core.SignalReply {
			reply = sig.Text
		}
	}
	return protocol.NewTextResult(call.ID, reply), nil
}
