package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"cube-adk/pkg/brain"
	"cube-adk/pkg/core"
	"cube-adk/pkg/engine"
	"cube-adk/pkg/runtime"
	"cube-adk/pkg/shelf"
	"cube-adk/pkg/vault"
)

// reportTool generates an HTML report and exposes it as an artifact.
type reportTool struct {
	mu        sync.Mutex
	artifacts []core.ArtifactDetail
}

func (t *reportTool) Identity() string { return "generate_report" }
func (t *reportTool) Brief() string {
	return "Generate an HTML report. Input: JSON {\"title\": \"...\", \"content\": \"...\"}"
}
func (t *reportTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":   map[string]any{"type": "string", "description": "report title"},
			"content": map[string]any{"type": "string", "description": "report body in markdown or plain text"},
		},
		"required": []string{"title", "content"},
	}
}

func (t *reportTool) Perform(_ context.Context, input string) (string, error) {
	var args struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := parseJSON(input, &args); err != nil {
		return "", err
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s</title></head>
<body><h1>%s</h1><div>%s</div></body></html>`, args.Title, args.Title, args.Content)

	t.mu.Lock()
	t.artifacts = []core.ArtifactDetail{{
		ID:   "report-1",
		Name: args.Title + ".html",
		MIME: "text/html",
		Data: []byte(html),
		Meta: map[string]string{"title": args.Title},
	}}
	t.mu.Unlock()

	return fmt.Sprintf("Report generated: %s (%d bytes)", args.Title+".html", len(html)), nil
}

func (t *reportTool) Artifacts() []core.ArtifactDetail {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.artifacts
	t.artifacts = nil
	return out
}

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
	sh := shelf.NewMemShelf()

	report := &reportTool{}

	agent := &engine.SoloAgent{
		Name:    "reporter",
		Persona: "You are a report assistant. When asked to write a report, use the generate_report tool. Reply in the user's language.",
		Brain:   b,
		Tools:   []core.Tool{report},
		Vault:   mv,
	}

	conv := runtime.NewConversation("artifact-demo", core.WithVault(mv), core.WithShelf(sh))
	conv.Append(core.Dialogue{Role: "user", Text: "Write a brief report about the benefits of AI in healthcare"})

	fmt.Println("=== Artifact Demo ===")
	ch, err := agent.Execute(context.Background(), conv)
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
			fmt.Printf(": [%s] %s", status, sig.Yield.Output)
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

func parseJSON(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}
