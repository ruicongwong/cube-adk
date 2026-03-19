package protocol

// PartKind identifies the type of content in a multimodal message part.
type PartKind int

const (
	PartText      PartKind = iota // plain text
	PartImage                     // image (URL or raw bytes)
	PartAudio                     // audio
	PartVideo                     // video
	PartFile                      // generic file
	PartReasoning                 // model reasoning/thinking
)

// PartMeta holds media metadata shared across non-text content parts.
type PartMeta struct {
	URL      string `json:"url,omitempty"`
	RawData  []byte `json:"raw_data,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

// ContentPart is a single piece of multimodal content within a Message.
type ContentPart struct {
	Kind PartKind `json:"kind"`
	Text string   `json:"text,omitempty"`
	PartMeta
}

// Usage holds token consumption reported by the model.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Message represents a single message in a conversation, supporting multimodal content.
type Message struct {
	Role       string         `json:"role"`                   // "system" | "user" | "assistant" | "tool"
	Content    []ContentPart  `json:"content,omitempty"`      // multimodal content parts
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`   // tool calls (assistant only)
	ToolCallID string         `json:"tool_call_id,omitempty"` // reference ID (tool role only)
	Name       string         `json:"name,omitempty"`         // optional sender name
	Reasoning  string         `json:"reasoning,omitempty"`    // model reasoning trace
	TokenUsage *Usage         `json:"token_usage,omitempty"`  // token consumption
	Extra      map[string]any `json:"extra,omitempty"`        // vendor-specific extensions
}

// TextOf concatenates all text parts in the message.
func (m *Message) TextOf() string {
	if len(m.Content) == 1 && m.Content[0].Kind == PartText {
		return m.Content[0].Text
	}
	var buf []byte
	for _, p := range m.Content {
		if p.Kind == PartText {
			buf = append(buf, p.Text...)
		}
	}
	return string(buf)
}

// NewTextMessage creates a simple text message.
func NewTextMessage(role, text string) *Message {
	return &Message{
		Role:    role,
		Content: []ContentPart{{Kind: PartText, Text: text}},
	}
}

// NewUserParts creates a user message with multiple content parts.
func NewUserParts(parts ...ContentPart) *Message {
	return &Message{
		Role:    "user",
		Content: parts,
	}
}

// WithToolCalls returns a copy of the message with tool calls attached.
func (m *Message) WithToolCalls(calls ...ToolCall) *Message {
	cp := *m
	cp.ToolCalls = calls
	return &cp
}
