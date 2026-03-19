// Package runtime provides convenience helpers built on top of core types.
package runtime

import "cube-adk/pkg/core"

// NewSession is a convenience re-export of core.NewSession.
func NewSession(id string, opts ...core.SessionOption) *core.Session {
	return core.NewSession(id, opts...)
}
