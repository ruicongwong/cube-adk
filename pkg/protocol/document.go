package protocol

// Document represents a piece of content for retrieval-augmented generation.
type Document struct {
	ID      string         `json:"id"`
	Content string         `json:"content"`
	Vector  []float64      `json:"vector,omitempty"`
	Score   float64        `json:"score,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}
