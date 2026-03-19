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
	Modified string
	Reason   string
}

// Checkpoint describes an action awaiting human approval.
type Checkpoint struct {
	Agent  string
	Kind   string
	Tool   string
	Input  string
	Signal Signal
}

// Gate blocks execution until a human provides a verdict.
type Gate interface {
	Check(ctx context.Context, cp Checkpoint) (Review, error)
}

// Policy decides whether a checkpoint needs human review.
type Policy interface {
	NeedsReview(cp Checkpoint) bool
}
