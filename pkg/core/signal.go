package core

import (
	"cube-adk/pkg/protocol"
	"time"
)

// SignalKind identifies the type of event in an agent execution stream.
type SignalKind int

const (
	SignalThink    SignalKind = iota // LLM reasoning step
	SignalInvoke                    // tool invocation request
	SignalYield                     // tool execution result
	SignalReply                     // final text reply
	SignalHandoff                   // transfer to another agent
	SignalFault                     // error
	SignalRecall                    // memory recall
	SignalPlan                      // deep agent: task decomposition plan
	SignalSynth                     // deep agent: synthesis of sub-results
	SignalArtifact                  // artifact produced by agent or tool
	SignalGate                      // HITL gate check in progress
)

func (k SignalKind) String() string {
	switch k {
	case SignalThink:
		return "think"
	case SignalInvoke:
		return "invoke"
	case SignalYield:
		return "yield"
	case SignalReply:
		return "reply"
	case SignalHandoff:
		return "handoff"
	case SignalFault:
		return "fault"
	case SignalRecall:
		return "recall"
	case SignalPlan:
		return "plan"
	case SignalSynth:
		return "synth"
	case SignalArtifact:
		return "artifact"
	case SignalGate:
		return "gate"
	default:
		return "unknown"
	}
}

// Signal is the event unit emitted during agent execution.
type Signal struct {
	Kind     SignalKind
	Source   string               // agent identity that produced this signal
	Text     string               // human-readable content (think / reply / fault)
	Invoke   *protocol.ToolCall   // tool invocation request
	Yield    *protocol.ToolResult // tool execution result
	Handoff  string               // target agent identity for handoff
	Recall   *RecallDetail
	Plan     *PlanDetail
	Artifact *ArtifactDetail
	TraceID  string
	SpanID   string
}

// Usage holds token consumption reported by the LLM.
type Usage = protocol.Usage

// RecallDetail describes memory recall results.
type RecallDetail struct {
	Query     string
	Fragments []Fragment
}

// Fragment is a piece of recalled memory.
type Fragment struct {
	Content string
	Score   float64
	Source  string
}

// PlanDetail describes a deep agent's task decomposition.
type PlanDetail struct {
	Tasks []SubTask
}

// SubTask is a decomposed unit of work in a deep agent plan.
type SubTask struct {
	ID          string
	Description string
	Result      string
	Done        bool
}

// ArtifactDetail describes a rich output produced during execution.
type ArtifactDetail struct {
	ID   string
	Name string
	MIME string
	Data []byte
	Meta map[string]string
}

// Scope defines the lifetime tier of a memory entry.
type Scope int

const (
	ScopeWorking Scope = iota
	ScopeShort
	ScopeLong
)

// Entry is a memory record to be stored in a Vault.
type Entry struct {
	Scope   Scope
	Tag     string
	Content string
	Meta    map[string]string
}

// Filter selects which memory entries to forget.
type Filter struct {
	Scope  *Scope
	Tag    string
	Before time.Time
}
