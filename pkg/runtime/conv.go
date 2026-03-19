// Package runtime provides convenience helpers built on top of core types.
package runtime

import "cube-adk/pkg/core"

// NewState is a convenience re-export of core.NewState.
func NewState(id string, opts ...core.StateOption) *core.State {
	return core.NewState(id, opts...)
}
