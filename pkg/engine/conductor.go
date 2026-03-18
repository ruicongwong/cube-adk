package engine

import (
	"context"
	"fmt"

	"cube-adk/pkg/core"
)

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
	return &Conductor{Name: name, Agents: m, EntryAgent: entry}
}

func (c *Conductor) Identity() string { return c.Name }

func (c *Conductor) Execute(ctx context.Context, conv *core.Conversation) (<-chan core.Signal, error) {
	ch := make(chan core.Signal, 16)
	go c.run(ctx, conv, ch)
	return ch, nil
}

func (c *Conductor) run(ctx context.Context, conv *core.Conversation, ch chan<- core.Signal) {
	defer close(ch)

	var span core.Span
	if c.Tracer != nil {
		ctx, span = c.Tracer.Start(ctx, "agent.conductor."+c.Name, core.SpanAgent)
		defer span.End(nil)
	}

	current := c.EntryAgent
	visited := make(map[string]int)

	for {
		if ctx.Err() != nil {
			return
		}

		agent, ok := c.Agents[current]
		if !ok {
			ch <- core.Signal{Kind: core.SignalFault, Source: c.Name,
				Text: fmt.Sprintf("unknown agent: %s", current)}
			return
		}

		visited[current]++
		if visited[current] > 10 {
			ch <- core.Signal{Kind: core.SignalFault, Source: c.Name,
				Text: fmt.Sprintf("agent %s exceeded max handoff visits", current)}
			return
		}

		sub, err := agent.Execute(ctx, conv)
		if err != nil {
			ch <- core.Signal{Kind: core.SignalFault, Source: c.Name, Text: err.Error()}
			return
		}

		handoff := ""
		for s := range sub {
			ch <- s
			if s.Kind == core.SignalHandoff {
				handoff = s.Handoff
			}
		}

		if handoff == "" {
			return // no handoff, done
		}
		current = handoff
	}
}

// AsTool wraps an Agent as a Tool so it can be used by other agents.
func AsTool(agent core.Agent) core.Tool {
	return &agentTool{agent: agent}
}

type agentTool struct {
	agent     core.Agent
	artifacts []core.ArtifactDetail
}

func (t *agentTool) Identity() string       { return t.agent.Identity() }
func (t *agentTool) Brief() string          { return "Delegate to agent: " + t.agent.Identity() }
func (t *agentTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string", "description": "The task to delegate"},
		},
		"required": []string{"input"},
	}
}

func (t *agentTool) Perform(ctx context.Context, input string) (string, error) {
	t.artifacts = nil
	conv := core.NewConversation("delegate")
	conv.Append(core.Dialogue{Role: "user", Text: input})

	ch, err := t.agent.Execute(ctx, conv)
	if err != nil {
		return "", err
	}

	var reply string
	for sig := range ch {
		if sig.Kind == core.SignalReply {
			reply = sig.Text
		}
		if sig.Kind == core.SignalArtifact && sig.Artifact != nil {
			t.artifacts = append(t.artifacts, *sig.Artifact)
		}
	}
	return reply, nil
}

func (t *agentTool) Artifacts() []core.ArtifactDetail { return t.artifacts }
