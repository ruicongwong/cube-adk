package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cube-adk/pkg/core"
)

var spanCounter atomic.Int64

func nextID() string {
	return fmt.Sprintf("span-%d", spanCounter.Add(1))
}

// RecordedSpan holds the data captured by MemTracer.
type RecordedSpan struct {
	ID       string
	Name     string
	Kind     core.SpanKind
	Start    time.Time
	End      time.Time
	Attrs    map[string]any
	Err      error
	Children []string // child span IDs
}

// Duration returns the elapsed time of the span.
func (s RecordedSpan) Duration() time.Duration {
	return s.End.Sub(s.Start)
}

// TokenUsage extracts token usage from span attributes, if present.
func (s RecordedSpan) TokenUsage() *core.Usage {
	pt, ok1 := s.Attrs["llm.prompt_tokens"].(int)
	ct, ok2 := s.Attrs["llm.completion_tokens"].(int)
	tt, ok3 := s.Attrs["llm.total_tokens"].(int)
	if !ok1 && !ok2 && !ok3 {
		return nil
	}
	return &core.Usage{PromptTokens: pt, CompletionTokens: ct, TotalTokens: tt}
}

// TTFT extracts the time-to-first-token from span attributes, if present.
func (s RecordedSpan) TTFT() time.Duration {
	if ms, ok := s.Attrs["llm.ttft_ms"].(int64); ok {
		return time.Duration(ms) * time.Millisecond
	}
	return 0
}

// MemTracer records all spans in memory for inspection.
type MemTracer struct {
	mu    sync.Mutex
	spans []RecordedSpan
}

func NewMemTracer() *MemTracer {
	return &MemTracer{}
}

func (t *MemTracer) Start(ctx context.Context, name string, kind core.SpanKind) (context.Context, core.Span) {
	s := &memSpan{
		tracer: t,
		rec: RecordedSpan{
			ID:    nextID(),
			Name:  name,
			Kind:  kind,
			Start: time.Now(),
			Attrs: make(map[string]any),
		},
	}
	// Link to parent
	if parent := core.SpanFromCtx(ctx); parent != nil {
		if ps, ok := parent.(*memSpan); ok {
			ps.mu.Lock()
			ps.rec.Children = append(ps.rec.Children, s.rec.ID)
			ps.mu.Unlock()
		}
	}
	return core.CtxWithSpan(ctx, s), s
}

// Spans returns a copy of all recorded spans.
func (t *MemTracer) Spans() []RecordedSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]RecordedSpan, len(t.spans))
	copy(out, t.spans)
	return out
}

func (t *MemTracer) record(rec RecordedSpan) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = append(t.spans, rec)
}

type memSpan struct {
	tracer *MemTracer
	rec    RecordedSpan
	mu     sync.Mutex
	ended  bool
}

func (s *memSpan) ID() string { return s.rec.ID }

func (s *memSpan) End(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.ended = true
	s.rec.End = time.Now()
	s.rec.Err = err
	s.tracer.record(s.rec)
}

func (s *memSpan) SetAttr(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.Attrs[key] = val
}

func (s *memSpan) Child(name string, kind core.SpanKind) core.Span {
	child := &memSpan{
		tracer: s.tracer,
		rec: RecordedSpan{
			ID:    nextID(),
			Name:  name,
			Kind:  kind,
			Start: time.Now(),
			Attrs: make(map[string]any),
		},
	}
	s.mu.Lock()
	s.rec.Children = append(s.rec.Children, child.rec.ID)
	s.mu.Unlock()
	return child
}

// TraceSummary aggregates metrics across all recorded spans.
type TraceSummary struct {
	TotalDuration         time.Duration
	BrainCalls            int
	BrainDuration         time.Duration
	ToolCalls             int
	ToolDuration          time.Duration
	TotalPromptTokens     int
	TotalCompletionTokens int
	TotalTokens           int
	AvgTTFT               time.Duration
}

// Summary computes aggregated metrics from all recorded spans.
func (t *MemTracer) Summary() TraceSummary {
	spans := t.Spans()
	var s TraceSummary
	var ttftSum int64
	var ttftCount int

	for _, sp := range spans {
		switch sp.Kind {
		case core.SpanAgent:
			if d := sp.Duration(); d > s.TotalDuration {
				s.TotalDuration = d
			}
		case core.SpanBrain:
			s.BrainCalls++
			s.BrainDuration += sp.Duration()
			if u := sp.TokenUsage(); u != nil {
				s.TotalPromptTokens += u.PromptTokens
				s.TotalCompletionTokens += u.CompletionTokens
				s.TotalTokens += u.TotalTokens
			}
			if ttft := sp.TTFT(); ttft > 0 {
				ttftSum += int64(ttft)
				ttftCount++
			}
		case core.SpanTool:
			s.ToolCalls++
			s.ToolDuration += sp.Duration()
		}
	}
	if ttftCount > 0 {
		s.AvgTTFT = time.Duration(ttftSum / int64(ttftCount))
	}
	return s
}
