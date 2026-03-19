package callback

import "cube-adk/pkg/component"

// RunInfo describes the context of a component execution for callback handlers.
type RunInfo struct {
	Name      string              // component or agent name
	Kind      string              // "agent", "model", "tool", etc.
	Component component.ComponentKind
	Extra     map[string]any
}
