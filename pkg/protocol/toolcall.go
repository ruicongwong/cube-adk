package protocol

import "fmt"

// ToolCall represents a tool invocation request from the model.
type ToolCall struct {
	ID   string `json:"id"`
	Kind string `json:"type"` // typically "function"
	Name string `json:"name"`
	Args string `json:"arguments"` // JSON-encoded arguments
}

// ToolSpec describes a tool's interface for model consumption.
type ToolSpec struct {
	Name   string         `json:"name"`
	Desc   string         `json:"description"`
	Schema map[string]any `json:"parameters,omitempty"` // JSON Schema
	Extra  map[string]any `json:"extra,omitempty"`      // vendor extensions
}

// ToolResult holds the output of a tool execution.
type ToolResult struct {
	CallID  string        `json:"tool_call_id"`
	Content []ContentPart `json:"content,omitempty"`
	Failed  bool          `json:"failed,omitempty"`
}

// NewTextResult creates a successful text-only tool result.
func NewTextResult(callID, text string) ToolResult {
	return ToolResult{
		CallID:  callID,
		Content: []ContentPart{{Kind: PartText, Text: text}},
	}
}

// NewErrorResult creates a failed tool result from an error.
func NewErrorResult(callID string, err error) ToolResult {
	return ToolResult{
		CallID:  callID,
		Content: []ContentPart{{Kind: PartText, Text: fmt.Sprintf("error: %v", err)}},
		Failed:  true,
	}
}

// TextOf concatenates all text content parts in the result.
func (r ToolResult) TextOf() string {
	if len(r.Content) == 1 && r.Content[0].Kind == PartText {
		return r.Content[0].Text
	}
	var buf []byte
	for _, p := range r.Content {
		if p.Kind == PartText {
			buf = append(buf, p.Text...)
		}
	}
	return string(buf)
}
