package core

import "context"

// Verdict is the human decision on a checkpoint.
type Verdict int

const (
	Approve Verdict = iota
	Reject
	Modify
)

// Review is the human's response to a checkpoint.
type Review struct {
	Verdict  Verdict
	Modified string // non-empty when Verdict == Modify
	Reason   string
}

// Checkpoint describes an action awaiting human approval.
type Checkpoint struct {
	Agent  string // agent identity
	Kind   string // "tool" | "handoff" | "plan" | "reply"
	Tool   string // tool name (when Kind == "tool")
	Input  string // content to review (args / plan / reply text)
	Signal Signal // the original signal being gated
}

// Gate blocks execution until a human provides a verdict.
type Gate interface {
	Check(ctx context.Context, cp Checkpoint) (Review, error)
}

// Policy decides whether a checkpoint needs human review.
type Policy interface {
	NeedsReview(cp Checkpoint) bool
}
