package engine

import (
	"context"
	"strings"
	"sync"

	"cube-adk/pkg/callback"
	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
)

// ParallelAgent executes multiple agents concurrently and merges their results.
type ParallelAgent struct {
	Name   string
	Agents []core.Agent
	Merge  func(results map[string][]core.Signal) string // optional merge function
	Tracer core.Tracer
}

func (p *ParallelAgent) Identity() string { return p.Name }

func (p *ParallelAgent) Execute(ctx context.Context, sess *core.Session) (<-chan core.Signal, error) {
	ch := make(chan core.Signal, 16)
	go p.run(ctx, sess, ch)
	return ch, nil
}

func (p *ParallelAgent) run(ctx context.Context, sess *core.Session, ch chan<- core.Signal) {
	defer close(ch)

	info := callback.RunInfo{Name: p.Name, Kind: "agent", Component: component.KindModel}
	ctx = callback.OnStart(ctx, info, sess)
	defer func() { callback.OnEnd(ctx, info, nil) }()

	var span core.Span
	if p.Tracer != nil {
		ctx, span = p.Tracer.Start(ctx, "agent.parallel."+p.Name, core.SpanAgent)
		defer span.End(nil)
	}

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

			var sigs []core.Signal
			for s := range sub {
				sigs = append(sigs, s)
			}

			mu.Lock()
			results[a.Identity()] = sigs
			mu.Unlock()
		}(agent)
	}

	wg.Wait()

	// Forward all signals from all agents
	for _, agent := range p.Agents {
		for _, s := range results[agent.Identity()] {
			ch <- s
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
		ch <- core.Signal{Kind: core.SignalReply, Source: p.Name, Text: mergedText}
	}
}
