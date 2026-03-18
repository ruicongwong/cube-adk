package main

import (
	"context"
	"fmt"
	"os"

	"cube-adk/pkg/brain"
	"cube-adk/pkg/core"
	"cube-adk/pkg/engine"
	"cube-adk/pkg/runtime"
	"cube-adk/pkg/tool"
	"cube-adk/pkg/vault"
)

func main() {
	endpoint := envOr("OPENAI_ENDPOINT", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	secret := envOr("OPENAI_API_KEY", "demo")
	model := envOr("OPENAI_MODEL", "qwen3.5-plus")

	if secret == "" {
		fmt.Println("Please set OPENAI_API_KEY")
		os.Exit(1)
	}

	b := brain.NewOAI(endpoint, secret, model)
	mv := vault.NewMemVault()

	ddg := tool.NewDuckDuckGoTool()

	// --- Chain example: researcher → writer ---
	fmt.Println("=== Chain Agent Demo ===")

	researcher := &engine.SoloAgent{
		Name:    "researcher",
		Persona: "You are a research assistant. Use the ddg_search tool to find relevant information, then summarize your findings.",
		Brain:   b,
		Tools:   []core.Tool{ddg},
		Vault:   mv,
	}

	writer := &engine.SoloAgent{
		Name:    "writer",
		Persona: "You are a professional writer. Take the research provided and write a well-structured article. Reply in the user's language.",
		Brain:   b,
		Tools:   nil,
		Vault:   mv,
	}

	chain := &engine.ChainAgent{
		Name:   "research-write-chain",
		Agents: []core.Agent{researcher, writer},
	}

	conv := runtime.NewConversation("team-demo", core.WithVault(mv))
	conv.Append(core.Dialogue{Role: "user", Text: "Write an article about the latest AI developments"})

	ch, err := chain.Execute(context.Background(), conv)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	printSignals(ch)

	// --- Conductor example: handoff between agents ---
	fmt.Println("\n=== Conductor Demo ===")

	researcherWithHandoff := &engine.SoloAgent{
		Name:    "researcher",
		Persona: "You are a research assistant. After gathering information, hand off to the writer agent to produce the final article.",
		Brain:   b,
		Tools:   []core.Tool{ddg},
		Vault:   mv,
	}

	conductor := engine.NewConductor("team", "researcher", researcherWithHandoff, writer)

	conv2 := runtime.NewConversation("conductor-demo", core.WithVault(mv))
	conv2.Append(core.Dialogue{Role: "user", Text: "Research and write about quantum computing breakthroughs"})

	ch2, err := conductor.Execute(context.Background(), conv2)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	printSignals(ch2)

	// --- Deep Agent example ---
	fmt.Println("\n=== Deep Agent Demo ===")

	deep := &engine.DeepAgent{
		Name:     "deep-researcher",
		Persona:  "You are a thorough research assistant. Break complex tasks into subtasks and solve them step by step. Reply in the user's language.",
		Brain:    b,
		Tools:    []core.Tool{ddg},
		Vault:    mv,
		MaxDepth: 2,
	}

	conv3 := runtime.NewConversation("deep-demo", core.WithVault(mv))
	conv3.Append(core.Dialogue{Role: "user", Text: "Compare the AI strategies of major tech companies in 2025"})

	ch3, err := deep.Execute(context.Background(), conv3)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	printSignals(ch3)
}

func printSignals(ch <-chan core.Signal) {
	for sig := range ch {
		fmt.Printf("[%s] %s", sig.Kind, sig.Source)
		switch sig.Kind {
		case core.SignalThink, core.SignalReply, core.SignalFault:
			fmt.Printf(": %s", sig.Text)
		case core.SignalInvoke:
			fmt.Printf(": %s(%s)", sig.Invoke.Name, sig.Invoke.Args)
		case core.SignalYield:
			status := "ok"
			if sig.Yield.Failed {
				status = "FAILED"
			}
			fmt.Printf(": [%s] %s", status, sig.Yield.Output)
		case core.SignalHandoff:
			fmt.Printf(": → %s", sig.Handoff)
		case core.SignalRecall:
			fmt.Printf(": query=%q, %d fragments", sig.Recall.Query, len(sig.Recall.Fragments))
		case core.SignalPlan:
			fmt.Printf(": %s", sig.Text)
		case core.SignalSynth:
			fmt.Printf(": %s", sig.Text)
		case core.SignalArtifact:
			fmt.Printf(": [%s] %s (%d bytes)", sig.Artifact.MIME, sig.Artifact.Name, len(sig.Artifact.Data))
		case core.SignalGate:
			fmt.Printf(": waiting for approval — %s", sig.Text)
		}
		fmt.Println()
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
