package gate

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"cube-adk/pkg/core"
)

// CLIGate prompts the user via stdin/stdout for approval.
type CLIGate struct{}

func NewCLIGate() *CLIGate { return &CLIGate{} }

func (g *CLIGate) Check(_ context.Context, cp core.Checkpoint) (core.Review, error) {
	fmt.Printf("\n=== GATE: %s ===\n", cp.Kind)
	fmt.Printf("Agent:  %s\n", cp.Agent)
	if cp.Tool != "" {
		fmt.Printf("Tool:   %s\n", cp.Tool)
	}
	fmt.Printf("Input:  %s\n", cp.Input)
	fmt.Print("Action [approve/reject/modify]: ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))

	switch line {
	case "reject", "r":
		fmt.Print("Reason: ")
		reason, _ := reader.ReadString('\n')
		return core.Review{Verdict: core.Reject, Reason: strings.TrimSpace(reason)}, nil
	case "modify", "m":
		fmt.Print("Modified input: ")
		modified, _ := reader.ReadString('\n')
		return core.Review{Verdict: core.Modify, Modified: strings.TrimSpace(modified)}, nil
	default:
		return core.Review{Verdict: core.Approve}, nil
	}
}
