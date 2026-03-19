package engine

import (
	"context"
	"strings"
	"sync"

	"cube-adk/pkg/core"
	"cube-adk/pkg/protocol"
)

// ParallelAgent executes multiple agents concurrently and merges their results.
type ParallelAgent struct {
	Name   string
	Agents []core.Agent
	Merge  func(results map[string][]core.Signal) string // optional merge function
	Tracer core.Tracer
}

func (p *ParallelAgent) Identity() string { return p.Name }

func (p *ParallelAgent) Execute(ctx context.Context, state *core.State) (*protocol.StreamReader[core.Signal], error) {
	r, w := protocol.Pipe[core.Signal](16)
	go p.run(ctx, state, w)
	return r, nil
}

func (p *ParallelAgent) run(ctx context.Context, sess *core.State, w *protocol.StreamWriter[core.Signal]) {
	defer w.Finish(nil)

	ctx, hooks := beginHooks(ctx, p.Tracer, p.Name, "agent.parallel."+p.Name, core.SpanAgent, sess)
	defer hooks.End(ctx, nil)

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make(map[string][]core.Signal)
	)

	for _, agent := range p.Agents {
		wg.Add(1)
		go func(a core.Agent) {
			defer wg.Done()

			sub, err := a.Execute(ctx, sess)
			if err != nil {
				mu.Lock()
				results[a.Identity()] = []core.Signal{
					{Kind: core.SignalFault, Source: a.Identity(), Text: err.Error()},
				}
				mu.Unlock()
				return
			}

			sigs, _ := protocol.CollectAll(sub)

			mu.Lock()
			results[a.Identity()] = sigs
			mu.Unlock()
		}(agent)
	}

	wg.Wait()

	// Forward all signals from all agents
	for _, agent := range p.Agents {
		for _, s := range results[agent.Identity()] {
			_ = w.Send(s)
		}
	}

	// Merge replies
	var mergedText string
	if p.Merge != nil {
		mergedText = p.Merge(results)
	} else {
		var parts []string
		for _, agent := range p.Agents {
			for _, s := range results[agent.Identity()] {
				if s.Kind == core.SignalReply {
					parts = append(parts, s.Text)
				}
			}
		}
		mergedText = strings.Join(parts, "\n\n")
	}

	if mergedText != "" {
		_ = w.Send(core.Signal{Kind: core.SignalReply, Source: p.Name, Text: mergedText})
	}
}
