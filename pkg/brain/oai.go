package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"cube-adk/pkg/core"
)

// OAI implements core.Brain using an OpenAI-compatible API.
type OAI struct {
	Endpoint   string // e.g. "https://api.openai.com/v1"
	Secret     string
	ModelID    string
	HTTPClient *http.Client
}

func NewOAI(endpoint, secret, model string) *OAI {
	return &OAI{
		Endpoint:   endpoint,
		Secret:     secret,
		ModelID:    model,
		HTTPClient: http.DefaultClient,
	}
}

// --- OpenAI request/response types ---

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string          `json:"type"`
	Function oaiToolFuncDef  `json:"function"`
}

type oaiToolFuncDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type oaiRequest struct {
	Model    string       `json:"model"`
	Messages []oaiMessage `json:"messages"`
	Tools    []oaiTool    `json:"tools,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type oaiResponse struct {
	Choices []struct {
		Message oaiMessage `json:"message"`
	} `json:"choices"`
	Usage *oaiUsage `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Think sends dialogue + tools to the LLM and returns the assistant response.
func (o *OAI) Think(ctx context.Context, dialogue []core.Dialogue, coreTools []core.Tool) (*core.Dialogue, error) {
	msgs := make([]oaiMessage, 0, len(dialogue))
	for _, d := range dialogue {
		m := oaiMessage{Role: d.Role, Content: d.Text, ToolCallID: d.InvokeRef}
		for _, inv := range d.Invocations {
			m.ToolCalls = append(m.ToolCalls, oaiToolCall{
				ID:   inv.ID,
				Type: "function",
				Function: oaiToolFunction{Name: inv.Name, Arguments: inv.Args},
			})
		}
		msgs = append(msgs, m)
	}

	var tools []oaiTool
	for _, t := range coreTools {
		params := t.Schema()
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, oaiTool{
			Type: "function",
			Function: oaiToolFuncDef{
				Name:        t.Identity(),
				Description: t.Brief(),
				Parameters:  params,
			},
		})
	}

	req := oaiRequest{Model: o.ModelID, Messages: msgs, Tools: tools}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("oai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.Secret)

	t0 := time.Now()
	resp, err := o.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("oai: http: %w", err)
	}
	ttft := time.Since(t0)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oai: read body: %w", err)
	}

	var oaiResp oaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("oai: unmarshal response: %w", err)
	}
	if oaiResp.Error != nil {
		return nil, fmt.Errorf("oai: api error: %s", oaiResp.Error.Message)
	}
	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("oai: empty choices")
	}

	msg := oaiResp.Choices[0].Message
	d := &core.Dialogue{Role: msg.Role, Text: msg.Content, TTFT: ttft}
	for _, tc := range msg.ToolCalls {
		d.Invocations = append(d.Invocations, core.InvokeDetail{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}
	if oaiResp.Usage != nil {
		d.Usage = &core.Usage{
			PromptTokens:     oaiResp.Usage.PromptTokens,
			CompletionTokens: oaiResp.Usage.CompletionTokens,
			TotalTokens:      oaiResp.Usage.TotalTokens,
		}
	}
	return d, nil
}
