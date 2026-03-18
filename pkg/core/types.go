package core

import "time"

// SignalKind identifies the type of event in an agent execution stream.
type SignalKind int

const (
	SignalThink   SignalKind = iota // LLM reasoning step
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
	Kind    SignalKind
	Source  string       // agent identity that produced this signal
	Text    string       // human-readable content (think / reply / fault)
	Invoke  *InvokeDetail
	Yield   *YieldDetail
	Handoff string       // target agent identity for handoff
	Recall   *RecallDetail
	Plan     *PlanDetail     // deep agent plan
	Artifact *ArtifactDetail // produced artifact
	TraceID  string          // optional trace correlation
	SpanID   string          // optional span correlation
}

// Usage holds token consumption reported by the LLM.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Dialogue represents a single message in a conversation.
type Dialogue struct {
	Role        string         // "system" | "user" | "assistant" | "tool"
	Text        string
	Invocations []InvokeDetail // tool calls made by assistant
	InvokeRef   string         // tool_call_id for role=tool
	Usage       *Usage         // LLM token usage (assistant messages only)
	TTFT        time.Duration  // time to first token (assistant messages only)
}

// InvokeDetail describes a tool invocation request from the LLM.
type InvokeDetail struct {
	ID   string // tool_call_id
	Name string // tool name
	Args string // JSON arguments
}

// YieldDetail describes the result of a tool execution.
type YieldDetail struct {
	RefID  string // corresponding tool_call_id
	Output string
	Failed bool
}

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
	Result      string // filled after execution
	Done        bool
}

// ArtifactDetail describes a rich output produced during execution.
type ArtifactDetail struct {
	ID   string            // unique identifier
	Name string            // human-readable name ("report.html")
	MIME string            // MIME type ("text/html", "image/png", "application/json")
	Data []byte            // raw content
	Meta map[string]string // optional metadata
}

// Scope defines the lifetime tier of a memory entry.
type Scope int

const (
	ScopeWorking Scope = iota // current task context
	ScopeShort                // session-level
	ScopeLong                 // persistent across sessions
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
