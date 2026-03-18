package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cube-adk/pkg/core"
)

// SoloAgent implements a ReAct-style single agent with optional memory, gating, and tracing.
type SoloAgent struct {
	Name      string
	Persona   string
	Brain     core.Brain
	Tools     []core.Tool
	Vault     core.Vault
	StepLimit int
	Gate      core.Gate   // optional HITL gate
	Policy    core.Policy // optional gate policy
	Tracer    core.Tracer // optional tracer
}

func (a *SoloAgent) Identity() string { return a.Name }

func (a *SoloAgent) Execute(ctx context.Context, conv *core.Conversation) (<-chan core.Signal, error) {
	ch := make(chan core.Signal, 16)
	go a.run(ctx, conv, ch)
	return ch, nil
}

func (a *SoloAgent) run(ctx context.Context, conv *core.Conversation, ch chan<- core.Signal) {
	defer close(ch)

	// Agent-level trace span
	var span core.Span
	if a.Tracer != nil {
		ctx, span = a.Tracer.Start(ctx, "agent."+a.Name, core.SpanAgent)
		defer span.End(nil)
	}

	limit := a.StepLimit
	if limit <= 0 {
		limit = 10
	}

	toolMap := make(map[string]core.Tool, len(a.Tools))
	for _, t := range a.Tools {
		toolMap[t.Identity()] = t
	}

	dialogue := a.buildDialogue(ctx, conv, ch)

	for step := 0; step < limit; step++ {
		if ctx.Err() != nil {
			return
		}

		resp, err := a.Brain.Think(ctx, dialogue, a.Tools)
		if err != nil {
			emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
			return
		}

		// No tool calls → final reply
		if len(resp.Invocations) == 0 {
			replyText := resp.Text

			// Gate: review reply before emitting
			if a.gateNeeded("reply") {
				emit(ch, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "reply"})
				review, err := a.Gate.Check(ctx, core.Checkpoint{
					Agent: a.Name, Kind: "reply", Input: replyText,
				})
				if err == nil {
					switch review.Verdict {
					case core.Reject:
						// Force another LLM round
						dialogue = append(dialogue, core.Dialogue{Role: "user", Text: "Please try again. Reason: " + review.Reason})
						continue
					case core.Modify:
						replyText = review.Modified
					}
				}
			}

			emit(ch, core.Signal{Kind: core.SignalReply, Source: a.Name, Text: replyText})
			conv.Append(core.Dialogue{Role: "assistant", Text: replyText})
			if a.Vault != nil {
				_ = a.Vault.Append(ctx, core.Entry{
					Scope: core.ScopeWorking, Tag: "reply", Content: replyText,
				})
			}
			return
		}

		if resp.Text != "" {
			emit(ch, core.Signal{Kind: core.SignalThink, Source: a.Name, Text: resp.Text})
		}

		dialogue = append(dialogue, *resp)

		for _, inv := range resp.Invocations {
			emit(ch, core.Signal{
				Kind: core.SignalInvoke, Source: a.Name,
				Invoke: &core.InvokeDetail{ID: inv.ID, Name: inv.Name, Args: inv.Args},
			})

			// Check for handoff
			if inv.Name == "handoff" {
				var hArgs struct{ Target string }
				if err := json.Unmarshal([]byte(inv.Args), &hArgs); err == nil && hArgs.Target != "" {
					// Gate: review handoff
					if a.gateNeeded("handoff") {
						emit(ch, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "handoff:" + hArgs.Target})
						review, err := a.Gate.Check(ctx, core.Checkpoint{
							Agent: a.Name, Kind: "handoff", Tool: hArgs.Target, Input: inv.Args,
						})
						if err == nil && review.Verdict == core.Reject {
							yd := core.YieldDetail{RefID: inv.ID, Output: "handoff rejected: " + review.Reason, Failed: true}
							emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &yd})
							dialogue = append(dialogue, core.Dialogue{Role: "tool", InvokeRef: inv.ID, Text: yd.Output})
							continue
						}
					}
					emit(ch, core.Signal{Kind: core.SignalHandoff, Source: a.Name, Handoff: hArgs.Target})
					return
				}
			}

			t, ok := toolMap[inv.Name]
			if !ok {
				yd := core.YieldDetail{RefID: inv.ID, Output: fmt.Sprintf("unknown tool: %s", inv.Name), Failed: true}
				emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &yd})
				dialogue = append(dialogue, core.Dialogue{Role: "tool", InvokeRef: inv.ID, Text: yd.Output})
				continue
			}

			// Gate: review tool call before execution
			args := inv.Args
			if a.gateNeeded("tool") {
				cp := core.Checkpoint{Agent: a.Name, Kind: "tool", Tool: inv.Name, Input: args}
				if a.Policy.NeedsReview(cp) {
					emit(ch, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "tool:" + inv.Name})
					review, err := a.Gate.Check(ctx, cp)
					if err == nil {
						switch review.Verdict {
						case core.Reject:
							yd := core.YieldDetail{RefID: inv.ID, Output: "rejected by human: " + review.Reason, Failed: true}
							emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &yd})
							dialogue = append(dialogue, core.Dialogue{Role: "tool", InvokeRef: inv.ID, Text: yd.Output})
							continue
						case core.Modify:
							args = review.Modified
						}
					}
				}
			}

			output, err := t.Perform(ctx, args)
			failed := err != nil
			if failed {
				output = err.Error()
			}
			yd := core.YieldDetail{RefID: inv.ID, Output: output, Failed: failed}
			emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &yd})
			dialogue = append(dialogue, core.Dialogue{Role: "tool", InvokeRef: inv.ID, Text: output})

			// Check for artifacts
			if at, ok := t.(core.ArtifactTool); ok {
				for _, art := range at.Artifacts() {
					artCopy := art
					if conv.Shelf() != nil {
						_ = conv.Shelf().Store(ctx, artCopy)
					}
					emit(ch, core.Signal{Kind: core.SignalArtifact, Source: a.Name, Artifact: &artCopy})
				}
			}
		}
	}

	emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: "step limit reached"})
}

// gateNeeded returns true if gate and policy are configured and the kind matches.
func (a *SoloAgent) gateNeeded(kind string) bool {
	if a.Gate == nil || a.Policy == nil {
		return false
	}
	return a.Policy.NeedsReview(core.Checkpoint{Kind: kind})
}

func (a *SoloAgent) buildDialogue(ctx context.Context, conv *core.Conversation, ch chan<- core.Signal) []core.Dialogue {
	var dialogue []core.Dialogue

	if a.Persona != "" {
		dialogue = append(dialogue, core.Dialogue{Role: "system", Text: a.Persona})
	}

	if a.Vault != nil {
		history := conv.History()
		query := lastUserQuery(history)
		if query != "" {
			frags, err := a.Vault.Recall(ctx, query, 5)
			if err == nil && len(frags) > 0 {
				var parts []string
				for _, f := range frags {
					parts = append(parts, f.Content)
				}
				recallText := "Recalled from memory:\n" + strings.Join(parts, "\n---\n")
				dialogue = append(dialogue, core.Dialogue{Role: "system", Text: recallText})
				emit(ch, core.Signal{
					Kind: core.SignalRecall, Source: a.Name,
					Recall: &core.RecallDetail{Query: query, Fragments: frags},
				})
			}
		}
	}

	dialogue = append(dialogue, conv.History()...)
	return dialogue
}

func lastUserQuery(history []core.Dialogue) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Text
		}
	}
	return ""
}

func emit(ch chan<- core.Signal, s core.Signal) {
	ch <- s
}
