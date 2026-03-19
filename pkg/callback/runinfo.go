package callback

// RunInfo describes the context of a component execution for callback handlers.
type RunInfo struct {
	Name  string         // component or agent name
	Kind  string         // "agent", "model", "tool", etc.
	Extra map[string]any
}
