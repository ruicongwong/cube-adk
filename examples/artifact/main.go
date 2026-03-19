package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"cube-adk/pkg/brain"
	"cube-adk/pkg/component"
	"cube-adk/pkg/core"
	"cube-adk/pkg/engine"
	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
	"cube-adk/pkg/runtime"
	"cube-adk/pkg/shelf"
	"cube-adk/pkg/vault"
)

// reportTool generates an HTML report and exposes it as an artifact.
type reportTool struct{}

func (t *reportTool) Identity() string { return "generate_report" }
func (t *reportTool) Brief() string {
	return "Generate an HTML report. Input: JSON {\"title\": \"...\", \"content\": \"...\"}"
}
func (t *reportTool) Spec() protocol.ToolSpec {
	return protocol.ToolSpec{
		Name: "generate_report",
		Desc: t.Brief(),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "report title"},
				"content": map[string]any{"type": "string", "description": "report body in markdown or plain text"},
			},
			"required": []string{"title", "content"},
		},
	}
}

func (t *reportTool) Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error) {
	var args struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(call.Args), &args); err != nil {
		return protocol.NewErrorResult(call.ID, err), nil
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s</title></head>
<body><h1>%s</h1><div>%s</div></body></html>`, args.Title, args.Title, args.Content)

	return protocol.NewTextResult(call.ID, fmt.Sprintf("Report generated: %s (%d bytes)", args.Title+".html", len(html))), nil
}

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
	sh := shelf.NewMemShelf()

	report := &reportTool{}

	agent := &engine.SoloAgent{
		Name:    "reporter",
		Persona: "You are a report assistant. When asked to write a report, use the generate_report tool. Reply in the user's language.",
		Model:   m,
		Tools:   []component.Tool{report},
		Vault:   mv,
	}

	sess := runtime.NewSession("artifact-demo", core.WithVault(mv), core.WithShelf(sh))
	sess.Append(protocol.NewTextMessage("user", "Write a brief report about the benefits of AI in healthcare"))

	fmt.Println("=== Artifact Demo ===")
	ch, err := agent.Execute(context.Background(), sess)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

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
		case core.SignalArtifact:
			fmt.Printf(": [%s] %s (%d bytes)", sig.Artifact.MIME, sig.Artifact.Name, len(sig.Artifact.Data))
		}
		fmt.Println()
	}

	// Verify artifact was stored in shelf
	fmt.Println("\n=== Shelf Contents ===")
	arts, _ := sh.List(context.Background(), "", 10)
	for _, a := range arts {
		fmt.Printf("  %s: %s (%s)\n", a.ID, a.Name, a.MIME)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
