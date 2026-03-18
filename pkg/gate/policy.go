package gate

import "cube-adk/pkg/core"

// AllowAll is a Policy that never requires review.
type AllowAll struct{}

func (AllowAll) NeedsReview(_ core.Checkpoint) bool { return false }

// ToolPolicy gates specific tools by name.
type ToolPolicy struct {
	Names map[string]struct{}
}

func NewToolPolicy(names ...string) *ToolPolicy {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return &ToolPolicy{Names: m}
}

func (p *ToolPolicy) NeedsReview(cp core.Checkpoint) bool {
	if cp.Kind != "tool" {
		return false
	}
	_, ok := p.Names[cp.Tool]
	return ok
}

// KindPolicy gates by checkpoint kind ("tool", "handoff", "plan", "reply").
type KindPolicy struct {
	Kinds map[string]struct{}
}

func NewKindPolicy(kinds ...string) *KindPolicy {
	m := make(map[string]struct{}, len(kinds))
	for _, k := range kinds {
		m[k] = struct{}{}
	}
	return &KindPolicy{Kinds: m}
}

func (p *KindPolicy) NeedsReview(cp core.Checkpoint) bool {
	_, ok := p.Kinds[cp.Kind]
	return ok
}

// CompositePolicy combines multiple policies with OR logic.
type CompositePolicy struct {
	Policies []core.Policy
}

func NewCompositePolicy(policies ...core.Policy) *CompositePolicy {
	return &CompositePolicy{Policies: policies}
}

func (p *CompositePolicy) NeedsReview(cp core.Checkpoint) bool {
	for _, pol := range p.Policies {
		if pol.NeedsReview(cp) {
			return true
		}
	}
	return false
}
