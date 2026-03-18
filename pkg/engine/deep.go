package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cube-adk/pkg/core"
)

// DeepAgent implements a recursive plan-decompose-execute-synthesize pattern.
type DeepAgent struct {
	Name      string
	Persona   string
	Brain     core.Brain
	Tools     []core.Tool
	Vault     core.Vault
	MaxDepth  int
	StepLimit int
	Gate      core.Gate
	Policy    core.Policy
	Tracer    core.Tracer
}

func (a *DeepAgent) Identity() string { return a.Name }

func (a *DeepAgent) Execute(ctx context.Context, conv *core.Conversation) (<-chan core.Signal, error) {
	ch := make(chan core.Signal, 32)
	go a.run(ctx, conv, ch, 0)
	return ch, nil
}

func (a *DeepAgent) run(ctx context.Context, conv *core.Conversation, ch chan<- core.Signal, depth int) {
	defer close(ch)

	var span core.Span
	if a.Tracer != nil {
		ctx, span = a.Tracer.Start(ctx, "agent.deep."+a.Name, core.SpanAgent)
		defer span.End(nil)
	}

	maxDepth := a.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	stepLimit := a.StepLimit
	if stepLimit <= 0 {
		stepLimit = 10
	}

	plan, err := a.planPhase(ctx, conv, ch)
	if err != nil {
		emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
		return
	}

	if len(plan.Tasks) == 0 || depth >= maxDepth {
		a.reactFallback(ctx, conv, ch, stepLimit)
		return
	}

	emit(ch, core.Signal{
		Kind: core.SignalPlan, Source: a.Name,
		Plan: &plan, Text: formatPlan(plan),
	})

	// Gate: review plan before execution
	if a.deepGateNeeded("plan") {
		emit(ch, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "plan"})
		review, err := a.Gate.Check(ctx, core.Checkpoint{
			Agent: a.Name, Kind: "plan", Input: formatPlan(plan),
		})
		if err == nil && review.Verdict == core.Reject {
			emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: "plan rejected: " + review.Reason})
			return
		}
	}

	for i := range plan.Tasks {
		if ctx.Err() != nil {
			return
		}
		task := &plan.Tasks[i]
		emit(ch, core.Signal{
			Kind: core.SignalThink, Source: a.Name,
			Text: fmt.Sprintf("Executing subtask [%d/%d]: %s", i+1, len(plan.Tasks), task.Description),
		})
		result, err := a.executeSubtask(ctx, task, ch, depth+1)
		if err != nil {
			task.Result = "ERROR: " + err.Error()
		} else {
			task.Result = result
			task.Done = true
		}
	}

	finalAnswer, err := a.synthPhase(ctx, conv, plan, ch)
	if err != nil {
		emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
		return
	}

	emit(ch, core.Signal{Kind: core.SignalSynth, Source: a.Name, Text: "Synthesizing results..."})
	emit(ch, core.Signal{Kind: core.SignalReply, Source: a.Name, Text: finalAnswer})
	conv.Append(core.Dialogue{Role: "assistant", Text: finalAnswer})

	if a.Vault != nil {
		_ = a.Vault.Append(ctx, core.Entry{
			Scope: core.ScopeWorking, Tag: "deep-reply", Content: finalAnswer,
		})
	}
}

func (a *DeepAgent) planPhase(ctx context.Context, conv *core.Conversation, ch chan<- core.Signal) (core.PlanDetail, error) {
	history := conv.History()
	query := lastUserQuery(history)

	planPrompt := fmt.Sprintf(
		`You are a task planner. Analyze the following task and break it into smaller subtasks if it is complex.
Return a JSON array of subtask descriptions. If the task is simple enough to solve directly, return an empty array [].

Task: %s

Respond ONLY with a JSON array of strings, e.g.: ["subtask 1", "subtask 2", "subtask 3"]`, query)

	dialogue := []core.Dialogue{
		{Role: "system", Text: a.Persona},
		{Role: "user", Text: planPrompt},
	}

	resp, err := a.Brain.Think(ctx, dialogue, nil)
	if err != nil {
		return core.PlanDetail{}, fmt.Errorf("deep: plan phase: %w", err)
	}

	var descriptions []string
	text := strings.TrimSpace(resp.Text)
	if err := json.Unmarshal([]byte(text), &descriptions); err != nil {
		text = stripCodeBlock(text)
		if err := json.Unmarshal([]byte(text), &descriptions); err != nil {
			return core.PlanDetail{}, nil
		}
	}

	plan := core.PlanDetail{Tasks: make([]core.SubTask, len(descriptions))}
	for i, desc := range descriptions {
		plan.Tasks[i] = core.SubTask{
			ID: fmt.Sprintf("%s-sub-%d", a.Name, i+1), Description: desc,
		}
	}
	return plan, nil
}

func (a *DeepAgent) executeSubtask(ctx context.Context, task *core.SubTask, ch chan<- core.Signal, depth int) (string, error) {
	subConv := core.NewConversation(task.ID)
	subConv.Append(core.Dialogue{Role: "user", Text: task.Description})

	child := &DeepAgent{
		Name: task.ID, Persona: a.Persona, Brain: a.Brain,
		Tools: a.Tools, Vault: a.Vault, MaxDepth: a.MaxDepth,
		StepLimit: a.StepLimit, Gate: a.Gate, Policy: a.Policy, Tracer: a.Tracer,
	}

	subCh := make(chan core.Signal, 32)
	go child.run(ctx, subConv, subCh, depth)

	var result string
	for sig := range subCh {
		ch <- sig
		if sig.Kind == core.SignalReply {
			result = sig.Text
		}
	}
	return result, nil
}

func (a *DeepAgent) synthPhase(ctx context.Context, conv *core.Conversation, plan core.PlanDetail, ch chan<- core.Signal) (string, error) {
	history := conv.History()
	query := lastUserQuery(history)

	var sb strings.Builder
	sb.WriteString("Original task: " + query + "\n\nSubtask results:\n")
	for i, t := range plan.Tasks {
		status := "DONE"
		if !t.Done {
			status = "FAILED"
		}
		sb.WriteString(fmt.Sprintf("\n[%d] %s\nStatus: %s\nResult: %s\n", i+1, t.Description, status, t.Result))
	}
	sb.WriteString("\nPlease synthesize all subtask results into a comprehensive final answer for the original task.")

	dialogue := []core.Dialogue{
		{Role: "system", Text: a.Persona},
		{Role: "user", Text: sb.String()},
	}

	resp, err := a.Brain.Think(ctx, dialogue, nil)
	if err != nil {
		return "", fmt.Errorf("deep: synth phase: %w", err)
	}
	return resp.Text, nil
}

// reactFallback runs a simple ReAct loop when recursion bottoms out.
func (a *DeepAgent) reactFallback(ctx context.Context, conv *core.Conversation, ch chan<- core.Signal, stepLimit int) {
	toolMap := make(map[string]core.Tool, len(a.Tools))
	for _, t := range a.Tools {
		toolMap[t.Identity()] = t
	}

	var dialogue []core.Dialogue
	if a.Persona != "" {
		dialogue = append(dialogue, core.Dialogue{Role: "system", Text: a.Persona})
	}
	dialogue = append(dialogue, conv.History()...)

	for step := 0; step < stepLimit; step++ {
		if ctx.Err() != nil {
			return
		}

		resp, err := a.Brain.Think(ctx, dialogue, a.Tools)
		if err != nil {
			emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
			return
		}

		if len(resp.Invocations) == 0 {
			emit(ch, core.Signal{Kind: core.SignalReply, Source: a.Name, Text: resp.Text})
			conv.Append(core.Dialogue{Role: "assistant", Text: resp.Text})
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

			t, ok := toolMap[inv.Name]
			if !ok {
				yd := core.YieldDetail{RefID: inv.ID, Output: fmt.Sprintf("unknown tool: %s", inv.Name), Failed: true}
				emit(ch, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &yd})
				dialogue = append(dialogue, core.Dialogue{Role: "tool", InvokeRef: inv.ID, Text: yd.Output})
				continue
			}

			// Gate: review tool call
			args := inv.Args
			if a.deepGateNeeded("tool") {
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

			if at, ok := t.(core.ArtifactTool); ok {
				for _, art := range at.Artifacts() {
					artCopy := art
					emit(ch, core.Signal{Kind: core.SignalArtifact, Source: a.Name, Artifact: &artCopy})
				}
			}
		}
	}

	emit(ch, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: "step limit reached"})
}

func (a *DeepAgent) deepGateNeeded(kind string) bool {
	if a.Gate == nil || a.Policy == nil {
		return false
	}
	return a.Policy.NeedsReview(core.Checkpoint{Kind: kind})
}

func formatPlan(plan core.PlanDetail) string {
	var sb strings.Builder
	sb.WriteString("Plan:\n")
	for i, t := range plan.Tasks {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, t.Description))
	}
	return sb.String()
}

func stripCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
