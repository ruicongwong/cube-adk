package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// DeepAgent implements a recursive plan-decompose-execute-synthesize pattern.
type DeepAgent struct {
	Name      string
	Persona   string
	Model     component.Model
	Tools     []component.Tool
	Vault     core.Vault
	MaxDepth  int
	StepLimit int
	Gate      core.Gate
	Policy    core.Policy
	Tracer    core.Tracer

	extraTools []component.Tool
}

func (a *DeepAgent) InjectTools(tools ...component.Tool) {
	a.extraTools = append(a.extraTools, tools...)
}

func (a *DeepAgent) Identity() string { return a.Name }

func (a *DeepAgent) Execute(ctx context.Context, state *core.State) (*protocol.StreamReader[core.Signal], error) {
	r, w := protocol.Pipe[core.Signal](32)
	go a.run(ctx, state, w, 0)
	return r, nil
}

func (a *DeepAgent) run(ctx context.Context, sess *core.State, w *protocol.StreamWriter[core.Signal], depth int) {
	defer w.Finish(nil)

	ctx, hooks := beginHooks(ctx, a.Tracer, a.Name, "agent.deep."+a.Name, core.SpanAgent, sess)
	defer hooks.End(ctx, nil)

	maxDepth := a.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	stepLimit := a.StepLimit
	if stepLimit <= 0 {
		stepLimit = 10
	}

	plan, err := a.planPhase(ctx, sess)
	if err != nil {
		emit(w, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
		return
	}

	if len(plan.Tasks) == 0 || depth >= maxDepth {
		a.reactFallback(ctx, sess, w, stepLimit)
		return
	}

	emit(w, core.Signal{
		Kind: core.SignalPlan, Source: a.Name,
		Plan: &plan, Text: formatPlan(plan),
	})

	if a.deepGateNeeded("plan") {
		emit(w, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "plan"})
		review, err := a.Gate.Check(ctx, core.Checkpoint{
			Agent: a.Name, Kind: "plan", Input: formatPlan(plan),
		})
		if err == nil && review.Verdict == core.Reject {
			emit(w, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: "plan rejected: " + review.Reason})
			return
		}
	}

	for i := range plan.Tasks {
		if ctx.Err() != nil {
			return
		}
		task := &plan.Tasks[i]
		emit(w, core.Signal{
			Kind: core.SignalThink, Source: a.Name,
			Text: fmt.Sprintf("Executing subtask [%d/%d]: %s", i+1, len(plan.Tasks), task.Description),
		})
		result, err := a.executeSubtask(ctx, task, w, depth+1)
		if err != nil {
			task.Result = "ERROR: " + err.Error()
		} else {
			task.Result = result
			task.Done = true
		}
	}

	finalAnswer, err := a.synthPhase(ctx, sess, plan)
	if err != nil {
		emit(w, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
		return
	}

	emit(w, core.Signal{Kind: core.SignalSynth, Source: a.Name, Text: "Synthesizing results..."})
	emit(w, core.Signal{Kind: core.SignalReply, Source: a.Name, Text: finalAnswer})
	sess.Append(protocol.NewTextMessage("assistant", finalAnswer))

	if a.Vault != nil {
		_ = a.Vault.Append(ctx, core.Entry{
			Scope: core.ScopeWorking, Tag: "deep-reply", Content: finalAnswer,
		})
	}
}

func (a *DeepAgent) planPhase(ctx context.Context, sess *core.State) (core.PlanDetail, error) {
	history := sess.History()
	query := lastUserQuery(history)

	var memCtx string
	if a.Vault != nil && query != "" {
		frags, err := a.Vault.Recall(ctx, query, 5)
		if err == nil && len(frags) > 0 {
			var parts []string
			for _, f := range frags {
				parts = append(parts, f.Content)
			}
			memCtx = "\n\nRelevant memory:\n" + strings.Join(parts, "\n---\n")
		}
	}

	planPrompt := fmt.Sprintf(
		`You are a task planner. Analyze the following task and break it into smaller subtasks if it is complex.
Return a JSON array of subtask descriptions. If the task is simple enough to solve directly, return an empty array [].

Task: %s%s

Respond ONLY with a JSON array of strings, e.g.: ["subtask 1", "subtask 2", "subtask 3"]`, query, memCtx)

	msgs := []*protocol.Message{
		protocol.NewTextMessage("system", a.Persona),
		protocol.NewTextMessage("user", planPrompt),
	}

	resp, err := a.Model.Generate(ctx, msgs)
	if err != nil {
		return core.PlanDetail{}, fmt.Errorf("deep: plan phase: %w", err)
	}

	var descriptions []string
	text := strings.TrimSpace(resp.TextOf())
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

func (a *DeepAgent) executeSubtask(ctx context.Context, task *core.SubTask, w *protocol.StreamWriter[core.Signal], depth int) (string, error) {
	subSess := core.NewState(task.ID)
	subSess.Append(protocol.NewTextMessage("user", task.Description))

	child := &DeepAgent{
		Name: task.ID, Persona: a.Persona, Model: a.Model,
		Tools: a.Tools, Vault: a.Vault, MaxDepth: a.MaxDepth,
		StepLimit: a.StepLimit, Gate: a.Gate, Policy: a.Policy, Tracer: a.Tracer,
	}

	childR, childW := protocol.Pipe[core.Signal](32)
	go child.run(ctx, subSess, childW, depth)

	var result string
	for {
		sig, err := childR.Recv()
		if err != nil {
			break
		}
		_ = w.Send(sig)
		if sig.Kind == core.SignalReply {
			result = sig.Text
		}
	}
	return result, nil
}

func (a *DeepAgent) synthPhase(ctx context.Context, sess *core.State, plan core.PlanDetail) (string, error) {
	history := sess.History()
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

	msgs := []*protocol.Message{
		protocol.NewTextMessage("system", a.Persona),
		protocol.NewTextMessage("user", sb.String()),
	}

	resp, err := a.Model.Generate(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("deep: synth phase: %w", err)
	}
	return resp.TextOf(), nil
}

func (a *DeepAgent) reactFallback(ctx context.Context, sess *core.State, w *protocol.StreamWriter[core.Signal], stepLimit int) {
	allTools := append(a.Tools, a.extraTools...)
	toolMap := make(map[string]component.Tool, len(allTools))
	specs := make([]protocol.ToolSpec, 0, len(allTools))
	for _, t := range allTools {
		toolMap[t.Identity()] = t
		specs = append(specs, t.Spec())
	}

	var msgs []*protocol.Message
	if a.Persona != "" {
		msgs = append(msgs, protocol.NewTextMessage("system", a.Persona))
	}

	if a.Vault != nil {
		query := lastUserQuery(sess.History())
		if query != "" {
			frags, err := a.Vault.Recall(ctx, query, 5)
			if err == nil && len(frags) > 0 {
				var parts []string
				for _, f := range frags {
					parts = append(parts, f.Content)
				}
				recallText := "Recalled from memory:\n" + strings.Join(parts, "\n---\n")
				msgs = append(msgs, protocol.NewTextMessage("system", recallText))
				emit(w, core.Signal{
					Kind: core.SignalRecall, Source: a.Name,
					Recall: &core.RecallDetail{Query: query, Fragments: frags},
				})
			}
		}
	}

	msgs = append(msgs, sess.History()...)

	for step := 0; step < stepLimit; step++ {
		if ctx.Err() != nil {
			return
		}

		resp, err := a.Model.Generate(ctx, msgs, option.WithToolSpecs(specs...))
		if err != nil {
			emit(w, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: err.Error()})
			return
		}

		if len(resp.ToolCalls) == 0 {
			emit(w, core.Signal{Kind: core.SignalReply, Source: a.Name, Text: resp.TextOf()})
			sess.Append(protocol.NewTextMessage("assistant", resp.TextOf()))
			return
		}

		if text := resp.TextOf(); text != "" {
			emit(w, core.Signal{Kind: core.SignalThink, Source: a.Name, Text: text})
		}
		msgs = append(msgs, resp)

		for _, tc := range resp.ToolCalls {
			emit(w, core.Signal{
				Kind: core.SignalInvoke, Source: a.Name,
				Invoke: &protocol.ToolCall{ID: tc.ID, Kind: tc.Kind, Name: tc.Name, Args: tc.Args},
			})

			t, ok := toolMap[tc.Name]
			if !ok {
				result := protocol.NewErrorResult(tc.ID, fmt.Errorf("unknown tool: %s", tc.Name))
				emit(w, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &result})
				msgs = append(msgs, toolResultMsg(result))
				continue
			}

			callToRun := tc
			if a.deepGateNeeded("tool") {
				cp := core.Checkpoint{Agent: a.Name, Kind: "tool", Tool: tc.Name, Input: tc.Args}
				if a.Policy.NeedsReview(cp) {
					emit(w, core.Signal{Kind: core.SignalGate, Source: a.Name, Text: "tool:" + tc.Name})
					review, err := a.Gate.Check(ctx, cp)
					if err == nil {
						switch review.Verdict {
						case core.Reject:
							result := protocol.NewErrorResult(tc.ID, fmt.Errorf("rejected: %s", review.Reason))
							emit(w, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &result})
							msgs = append(msgs, toolResultMsg(result))
							continue
						case core.Modify:
							callToRun.Args = review.Modified
						}
					}
				}
			}

			result, _ := t.Run(ctx, callToRun)
			emit(w, core.Signal{Kind: core.SignalYield, Source: a.Name, Yield: &result})
			msgs = append(msgs, toolResultMsg(result))
		}
	}

	emit(w, core.Signal{Kind: core.SignalFault, Source: a.Name, Text: "step limit reached"})
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
