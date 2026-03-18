package core

import "context"

// Tool represents a callable capability that an agent can invoke.
type Tool interface {
	Identity() string
	Brief() string
	Schema() map[string]any
	Perform(ctx context.Context, input string) (string, error)
}

// QuickTool creates a Tool from a plain function.
type QuickTool struct {
	Name   string
	Desc   string
	Params map[string]any
	Fn     func(ctx context.Context, input string) (string, error)
}

func (t *QuickTool) Identity() string       { return t.Name }
func (t *QuickTool) Brief() string          { return t.Desc }
func (t *QuickTool) Schema() map[string]any { return t.Params }
func (t *QuickTool) Perform(ctx context.Context, input string) (string, error) {
	return t.Fn(ctx, input)
}

// RESTSpec declaratively describes an HTTP RESTful endpoint to be wrapped as a Tool.
type RESTSpec struct {
	Name        string
	Desc        string
	Method      string            // GET / POST / PUT / DELETE
	URL         string            // supports {param} path placeholders
	Headers     map[string]string
	QueryParams map[string]string
	BodyTpl     string            // JSON template with {{.field}} placeholders
	ResultPath  string            // dot-separated path to extract from JSON response
}

// ArtifactTool is optionally implemented by tools that produce artifacts.
type ArtifactTool interface {
	Tool
	// Artifacts returns artifacts produced by the last Perform call.
	Artifacts() []ArtifactDetail
}
