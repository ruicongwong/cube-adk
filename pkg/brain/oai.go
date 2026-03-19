package brain

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// OpenAIModel implements component.Model using an OpenAI-compatible API.
// Works with OpenAI, vLLM, and any compatible endpoint.
type OpenAIModel struct {
	Endpoint   string
	Secret     string
	ModelID    string
	HTTPClient *http.Client
}

// NewOpenAIModel creates a Model for OpenAI-compatible endpoints.
func NewOpenAIModel(endpoint, secret, model string) *OpenAIModel {
	return &OpenAIModel{
		Endpoint:   strings.TrimRight(endpoint, "/"),
		Secret:     secret,
		ModelID:    model,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// NewVLLMModel creates a Model for vLLM endpoints (typically no auth needed).
func NewVLLMModel(endpoint, model string) *OpenAIModel {
	return NewOpenAIModel(endpoint, "", model)
}

func (m *OpenAIModel) GetType() string { return "openai-compat" }

// --- OpenAI wire types ---

type oaiContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

type oaiMessage struct {
	Role       string  `json:"role"`
	Content    any     `json:"content,omitempty"` // string or []oaiContent
	ToolCalls  []oaiTC `json:"tool_calls,omitempty"`
	ToolCallID string  `json:"tool_call_id,omitempty"`
	Name       string  `json:"name,omitempty"`
}

type oaiTC struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Function oaiTCFunc `json:"function"`
}

type oaiTCFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiToolFunc `json:"function"`
}

type oaiToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	MaxTokens   *int         `json:"max_tokens,omitempty"`
	TopP        *float64     `json:"top_p,omitempty"`
	Stop        []string     `json:"stop,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type oaiChoice struct {
	Message oaiMessage `json:"message"`
	Delta   oaiMessage `json:"delta"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Generate sends messages to the model and returns a single response.
func (m *OpenAIModel) Generate(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.Message, error) {
	var mopts option.ModelOpts
	option.Apply(&mopts, opts...)

	req := m.buildRequest(msgs, &mopts, false)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("oai: marshal: %w", err)
	}

	body = m.applyVLLMOpts(body, opts...)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", m.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	m.setHeaders(httpReq)

	resp, err := m.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("oai: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oai: read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oai: status %d: %s", resp.StatusCode, string(respBody))
	}

	var oaiResp oaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("oai: unmarshal: %w", err)
	}
	if oaiResp.Error != nil {
		return nil, fmt.Errorf("oai: api: %s", oaiResp.Error.Message)
	}
	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("oai: empty choices")
	}

	return m.parseResponse(oaiResp.Choices[0].Message, oaiResp.Usage), nil
}

// Stream sends messages and returns a streaming reader of partial responses.
func (m *OpenAIModel) Stream(ctx context.Context, msgs []*protocol.Message, opts ...option.ModelOption) (*protocol.StreamReader[*protocol.Message], error) {
	var mopts option.ModelOpts
	option.Apply(&mopts, opts...)

	req := m.buildRequest(msgs, &mopts, true)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("oai: marshal: %w", err)
	}

	body = m.applyVLLMOpts(body, opts...)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", m.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	m.setHeaders(httpReq)

	resp, err := m.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("oai: http: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("oai: status %d: %s", resp.StatusCode, string(b))
	}

	reader, writer := protocol.Pipe[*protocol.Message](8)
	go m.readSSE(resp.Body, writer)
	return reader, nil
}

func (m *OpenAIModel) readSSE(body io.ReadCloser, w *protocol.StreamWriter[*protocol.Message]) {
	defer body.Close()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk oaiResponse
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		msg := m.parseResponse(chunk.Choices[0].Delta, chunk.Usage)
		if err := w.Send(msg); err != nil {
			break
		}
	}
	w.Finish(nil)
}

func (m *OpenAIModel) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if m.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+m.Secret)
	}
}

func (m *OpenAIModel) buildRequest(msgs []*protocol.Message, opts *option.ModelOpts, stream bool) oaiRequest {
	req := oaiRequest{
		Model:       m.ModelID,
		Stream:      stream,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		TopP:        opts.TopP,
		Stop:        opts.StopWords,
	}

	for _, msg := range msgs {
		req.Messages = append(req.Messages, marshalMessage(msg))
	}

	for _, spec := range opts.ToolSpecs {
		params := spec.Schema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		req.Tools = append(req.Tools, oaiTool{
			Type:     "function",
			Function: oaiToolFunc{Name: spec.Name, Description: spec.Desc, Parameters: params},
		})
	}

	return req
}

func marshalMessage(msg *protocol.Message) oaiMessage {
	om := oaiMessage{
		Role:       msg.Role,
		ToolCallID: msg.ToolCallID,
		Name:       msg.Name,
	}

	// Multimodal content
	hasNonText := false
	for _, p := range msg.Content {
		if p.Kind != protocol.PartText {
			hasNonText = true
			break
		}
	}

	if !hasNonText {
		// Simple text content
		om.Content = msg.TextOf()
	} else {
		// Multimodal content array
		var parts []oaiContent
		for _, p := range msg.Content {
			switch p.Kind {
			case protocol.PartText:
				parts = append(parts, oaiContent{Type: "text", Text: p.Text})
			case protocol.PartImage:
				url := p.URL
				if url == "" && len(p.RawData) > 0 {
					url = "data:" + p.MIMEType + ";base64," + encodeBase64(p.RawData)
				}
				parts = append(parts, oaiContent{
					Type:     "image_url",
					ImageURL: &struct{ URL string `json:"url"` }{URL: url},
				})
			}
		}
		om.Content = parts
	}

	for _, tc := range msg.ToolCalls {
		om.ToolCalls = append(om.ToolCalls, oaiTC{
			ID:   tc.ID,
			Type: "function",
			Function: oaiTCFunc{Name: tc.Name, Arguments: tc.Args},
		})
	}

	return om
}

func (m *OpenAIModel) parseResponse(msg oaiMessage, usage *oaiUsage) *protocol.Message {
	out := &protocol.Message{Role: msg.Role}

	// Parse content
	switch c := msg.Content.(type) {
	case string:
		if c != "" {
			out.Content = []protocol.ContentPart{{Kind: protocol.PartText, Text: c}}
		}
	case nil:
		// no content
	}

	for _, tc := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, protocol.ToolCall{
			ID:   tc.ID,
			Kind: tc.Type,
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}

	if usage != nil {
		out.TokenUsage = &protocol.Usage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
	}

	return out
}

// --- vLLM-specific options ---

// VLLMOpts holds vLLM-specific generation parameters.
type VLLMOpts struct {
	GuidedJSON        any
	GuidedRegex       string
	RepetitionPenalty float64
}

// WithGuidedJSON sets vLLM guided JSON decoding schema.
func WithGuidedJSON(schema any) option.ModelOption {
	return option.WrapImpl[option.ModelOpts, VLLMOpts](func(o *VLLMOpts) {
		o.GuidedJSON = schema
	})
}

// WithGuidedRegex sets vLLM guided regex pattern.
func WithGuidedRegex(pattern string) option.ModelOption {
	return option.WrapImpl[option.ModelOpts, VLLMOpts](func(o *VLLMOpts) {
		o.GuidedRegex = pattern
	})
}

// WithRepetitionPenalty sets vLLM repetition penalty.
func WithRepetitionPenalty(p float64) option.ModelOption {
	return option.WrapImpl[option.ModelOpts, VLLMOpts](func(o *VLLMOpts) {
		o.RepetitionPenalty = p
	})
}

func (m *OpenAIModel) applyVLLMOpts(body []byte, opts ...option.ModelOption) []byte {
	fns := option.ExtractImpl[option.ModelOpts, VLLMOpts](opts...)
	if len(fns) == 0 {
		return body
	}

	var vopts VLLMOpts
	for _, fn := range fns {
		fn(&vopts)
	}

	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return body
	}

	if vopts.GuidedJSON != nil {
		raw["guided_json"] = vopts.GuidedJSON
	}
	if vopts.GuidedRegex != "" {
		raw["guided_regex"] = vopts.GuidedRegex
	}
	if vopts.RepetitionPenalty > 0 {
		raw["repetition_penalty"] = vopts.RepetitionPenalty
	}

	out, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	return out
}

func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
