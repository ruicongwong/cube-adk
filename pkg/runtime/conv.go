// Package runtime provides convenience helpers built on top of core types.
package runtime

import "cube-adk/pkg/core"

// NewConversation is a convenience re-export of core.NewConversation.
func NewConversation(id string, opts ...core.ConvOption) *core.Conversation {
	return core.NewConversation(id, opts...)
}
