package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cube-adk/pkg/callback"
	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// SoloAgent implements a ReAct-style single agent with optional memory, gating, and tracing.
type SoloAgent struct {
	Name      string
	Persona   string
	Model     component.Model
	Tools     []component.Tool
	Vault     core.Vault
	StepLimit int
	Gate      core.Gate
	Policy    core.Policy
	Tracer    core.Tracer

	extraTools []component.Tool // injected by Conductor
}

// InjectTools appends tools managed by the framework (e.g. handoff).
func (a *SoloAgent) InjectTools(tools ...component.Tool) {
	a.extraTools = append(a.extraTools, tools...)
}

func (a *SoloAgent) Identity() string { return a.Name }

func (a *SoloAgent) Execute(ctx context.Context, sess *core.Session) (<-chan core.Signal, error) {
	ch := make(chan core.Signal, 16)
	go a.run(ctx, sess, ch)
	return ch, nil
}

func (a *SoloAgent) run(ctx context.Context, sess *core.Session, ch chan<- core.Signal) {
	defer close(ch)

	info := callback.RunInfo{Name: a.Name, Kind: "agent", Component: component.KindModel}
	ctx = callback.OnStart(ctx, info, sess)
	defer func() { callback.OnEnd(ctx, info, nil) }()

	var span core.Span
	if a.Tracer != nil {
		ctx, span = a.Tracer.Start(ctx, "agent."+a.Name, core.SpanAgent)
		defer span.End(nil)
	}

	limit := a.StepLimit
	if limit <= 0 {
		limit = 10
	}

	allTools := append(a.Tools, a.extraTools...)
	toolMap := make(map[string]component.Tool, len(allTools))
	specs := make([]protocol.ToolSpec, 0, len(allTools))
	for _, t := range allTools {
		toolMap[t.Identity()] = t
		specs = append(specs, t.Spec())
	}

	msgs := a.buildMessages(ctx, sess, ch)

	for step := 0; step < limit; step++ {
		if ctx.Err() != nil {
			return
		}

		resp, err := a.Model.Generate(ctx, msgs, option.WithToolSpecs(specs...))
		if err != nil {
			callback.OnError(ctx, info, err)
			emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
			return
		}

		// No tool calls → final reply
		if len(resp.ToolCalls) == 0 {
			replyText := resp.TextOf()

			if a.gateNeeded("reply") {
				emit(ch, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "reply"})
				review, err := a.Gate.Check(ctx, core.Checkpoint{
					Agent: a.Name, Kind: "reply", Input: replyText,
				})
				if err == nil {
					switch review.Verdict {
					case core.Reject:
						msgs = append(msgs, protocol.NewTextMessage("user", "Please try again. Reason: "+review.Reason))
						continue
					case core.Modify:
						replyText = review.Modified
					}
				}
			}

			emit(ch, core.Signal{Kind: core.SignalReply, Source: a.Name, Text: replyText})
			sess.Append(protocol.NewTextMessage("assistant", replyText))
			if a.Vault != nil {
				_ = a.Vault.Append(ctx, core.Entry{
					Scope: core.ScopeWorking, Tag: "reply", Content: replyText,
				})
			}
			return
		}

		if text := resp.TextOf(); text != "" {
			emit(ch, core.Signal{Kind: core.SignalThink, Source: a.Name, Text: text})
		}

		msgs = append(msgs, resp)

		for _, tc := range resp.ToolCalls {
			emit(ch, core.Signal{
				Kind: core.SignalInvoke, Source: a.Name,
				Invoke: &protocol.ToolCall{ID: tc.ID, Kind: tc.Kind, Name: tc.Name, Args: tc.Args},
			})

			// Check for handoff
			if tc.Name == "handoff" {
				var hArgs struct{ Target string }
				if err := json.Unmarshal([]byte(tc.Args), &hArgs); err == nil && hArgs.Target != "" {
					if a.gateNeeded("handoff") {
						emit(ch, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "handoff:" + hArgs.Target})
						review, err := a.Gate.Check(ctx, core.Checkpoint{
							Agent: a.Name, Kind: "handoff", Tool: hArgs.Target, Input: tc.Args,
						})
						if err == nil && review.Verdict == core.Reject {
							result := protocol.NewErrorResult(tc.ID, fmt.Errorf("handoff rejected: %s", review.Reason))
							emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &result})
							msgs = append(msgs, toolResultMsg(result))
							continue
						}
					}
					emit(ch, core.Signal{Kind: core.SignalHandoff, Source: a.Name, Handoff: hArgs.Target})
					return
				}
			}

			t, ok := toolMap[tc.Name]
			if !ok {
				result := protocol.NewErrorResult(tc.ID, fmt.Errorf("unknown tool: %s", tc.Name))
				emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &result})
				msgs = append(msgs, toolResultMsg(result))
				continue
			}

			// Gate: review tool call
			callToRun := tc
			if a.gateNeeded("tool") {
				cp := core.Checkpoint{Agent: a.Name, Kind: "tool", Tool: tc.Name, Input: tc.Args}
				if a.Policy.NeedsReview(cp) {
					emit(ch, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "tool:" + tc.Name})
					review, err := a.Gate.Check(ctx, cp)
					if err == nil {
						switch review.Verdict {
						case core.Reject:
							result := protocol.NewErrorResult(tc.ID, fmt.Errorf("rejected: %s", review.Reason))
							emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &result})
							msgs = append(msgs, toolResultMsg(result))
							continue
						case core.Modify:
							callToRun.Args = review.Modified
						}
					}
				}
			}

			result, _ := t.Run(ctx, callToRun)
			emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &result})
			msgs = append(msgs, toolResultMsg(result))
		}
	}

	emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: "step limit reached"})
}

func (a *SoloAgent) gateNeeded(kind string) bool {
	if a.Gate == nil || a.Policy == nil {
		return false
	}
	return a.Policy.NeedsReview(core.Checkpoint{Kind: kind})
}

func (a *SoloAgent) buildMessages(ctx context.Context, sess *core.Session, ch chan<- core.Signal) []*protocol.Message {
	var msgs []*protocol.Message

	if a.Persona != "" {
		msgs = append(msgs, protocol.NewTextMessage("system", a.Persona))
	}

	if a.Vault != nil {
		history := sess.History()
		query := lastUserQuery(history)
		if query != "" {
			frags, err := a.Vault.Recall(ctx, query, 5)
			if err == nil && len(frags) > 0 {
				var parts []string
				for _, f := range frags {
					parts = append(parts, f.Content)
				}
				recallText := "Recalled from memory:\n" + strings.Join(parts, "\n---\n")
				msgs = append(msgs, protocol.NewTextMessage("system", recallText))
				emit(ch, core.Signal{
					Kind: core.SignalRecall, Source: a.Name,
					Recall: &core.RecallDetail{Query: query, Fragments: frags},
				})
			}
		}
	}

	msgs = append(msgs, sess.History()...)
	return msgs
}

func lastUserQuery(history []*protocol.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].TextOf()
		}
	}
	return ""
}

func emit(ch chan<- core.Signal, s core.Signal) {
	ch <- s
}

// toolResultMsg creates a tool-role message from a ToolResult.
func toolResultMsg(r protocol.ToolResult) *protocol.Message {
	return &protocol.Message{
		Role:       "tool",
		ToolCallID: r.CallID,
		Content:    r.Content,
	}
}
