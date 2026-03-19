package main

import (
	"context"
	"fmt"
	"os"

	"cube-adk/pkg/brain"
	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
	"cube-adk/pkg/engine"
	"cube-adk/pkg/protocol"
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

	m := brain.NewOpenAIModel(endpoint, secret, model)
	mv := vault.NewMemVault()

	ddg := tool.NewDuckDuckGoTool()

	// --- Chain example: researcher → writer ---
	fmt.Println("=== Chain Agent Demo ===")

	researcher := &engine.SoloAgent{
		Name:    "researcher",
		Persona: "You are a research assistant. Use the ddg_search tool to find relevant information, then summarize your findings.",
		Model:   m,
		Tools:   []component.Tool{ddg},
		Vault:   mv,
	}

	writer := &engine.SoloAgent{
		Name:    "writer",
		Persona: "You are a professional writer. Take the research provided and write a well-structured article. Reply in the user's language.",
		Model:   m,
		Tools:   nil,
		Vault:   mv,
	}

	chain := &engine.ChainAgent{
		Name:   "research-write-chain",
		Agents: []core.Agent{researcher, writer},
	}

	sess := runtime.NewSession("team-demo", core.WithVault(mv))
	sess.Append(protocol.NewTextMessage("user", "Write an article about the latest AI developments"))

	ch, err := chain.Execute(context.Background(), sess)
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
		Model:   m,
		Tools:   []component.Tool{ddg},
		Vault:   mv,
	}

	conductor := engine.NewConductor("team", "researcher", researcherWithHandoff, writer)

	sess2 := runtime.NewSession("conductor-demo", core.WithVault(mv))
	sess2.Append(protocol.NewTextMessage("user", "Research and write about quantum computing breakthroughs"))

	ch2, err := conductor.Execute(context.Background(), sess2)
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
		Model:    m,
		Tools:    []component.Tool{ddg},
		Vault:    mv,
		MaxDepth: 2,
	}

	sess3 := runtime.NewSession("deep-demo", core.WithVault(mv))
	sess3.Append(protocol.NewTextMessage("user", "Compare the AI strategies of major tech companies in 2025"))

	ch3, err := deep.Execute(context.Background(), sess3)
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
			fmt.Printf(": [%s] %s", status, sig.Yield.TextOf())
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
