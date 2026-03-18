package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

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

	// Calculator tool
	calc := &core.QuickTool{
		Name: "calculator",
		Desc: "Evaluate a math expression. Input: JSON {\"expr\": \"2+3*4\"}",
		Params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expr": map[string]any{"type": "string", "description": "math expression"},
			},
			"required": []string{"expr"},
		},
		Fn: func(_ context.Context, input string) (string, error) {
			var args struct{ Expr string }
			if err := json.Unmarshal([]byte(input), &args); err != nil {
				return "", err
			}
			result, err := evalExpr(args.Expr)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%g", result), nil
		},
	}

	// Weather REST tool
	weather := tool.NewRESTTool(core.RESTSpec{
		Name:       "get_weather",
		Desc:       "Get current weather for a city. Input: JSON {\"city\": \"Beijing\"}",
		Method:     "GET",
		URL:        "https://api.weather.com/v1/city/{city}",
		Headers:    map[string]string{"X-API-Key": "demo"},
		ResultPath: "data.current",
	})

	// DuckDuckGo search tool
	ddg := tool.NewDuckDuckGoTool()

	agent := &engine.SoloAgent{
		Name:    "assistant",
		Persona: "You are a helpful assistant. Use tools when needed. Reply in the user's language.",
		Brain:   b,
		Tools:   []core.Tool{calc, weather, ddg},
		Vault:   mv,
	}

	conv := runtime.NewConversation("demo", core.WithVault(mv))
	conv.Append(core.Dialogue{Role: "user", Text: "马云"})

	fmt.Println("=== Solo Agent Demo ===")
	ch, err := agent.Execute(context.Background(), conv)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	printSignals(ch)
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
		case core.SignalRecall:
			fmt.Printf(": query=%q, %d fragments", sig.Recall.Query, len(sig.Recall.Fragments))
		case core.SignalPlan:
			fmt.Printf(": %s", sig.Text)
		case core.SignalSynth:
			fmt.Printf(": %s", sig.Text)
		case core.SignalHandoff:
			fmt.Printf(": → %s", sig.Handoff)
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

func evalExpr(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	p := &exprParser{input: expr}
	result := p.parseExpr()
	if p.err != nil {
		return 0, p.err
	}
	return result, nil
}

type exprParser struct {
	input string
	pos   int
	err   error
}

func (p *exprParser) parseExpr() float64 {
	result := p.parseTerm()
	for p.pos < len(p.input) && (p.input[p.pos] == '+' || p.input[p.pos] == '-') {
		op := p.input[p.pos]
		p.pos++
		right := p.parseTerm()
		if op == '+' {
			result += right
		} else {
			result -= right
		}
	}
	return result
}

func (p *exprParser) parseTerm() float64 {
	result := p.parseFactor()
	for p.pos < len(p.input) && (p.input[p.pos] == '*' || p.input[p.pos] == '/') {
		op := p.input[p.pos]
		p.pos++
		right := p.parseFactor()
		if op == '*' {
			result *= right
		} else {
			if right == 0 {
				p.err = fmt.Errorf("division by zero")
				return math.NaN()
			}
			result /= right
		}
	}
	return result
}

func (p *exprParser) parseFactor() float64 {
	if p.pos < len(p.input) && p.input[p.pos] == '(' {
		p.pos++
		result := p.parseExpr()
		if p.pos < len(p.input) && p.input[p.pos] == ')' {
			p.pos++
		}
		return result
	}
	start := p.pos
	if p.pos < len(p.input) && (p.input[p.pos] == '-' || p.input[p.pos] == '+') {
		p.pos++
	}
	for p.pos < len(p.input) && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9' || p.input[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		p.err = fmt.Errorf("unexpected char at pos %d", p.pos)
		return 0
	}
	val, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		p.err = err
		return 0
	}
	return val
}
