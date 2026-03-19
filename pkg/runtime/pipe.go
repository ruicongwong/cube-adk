package runtime

import (
	"cube-adk/pkg/core"
	"cube-adk/pkg/protocol"
)

// Collect drains a signal stream into a slice.
func Collect(r *protocol.StreamReader[core.Signal]) []core.Signal {
	out, _ := protocol.CollectAll(r)
	return out
}

// Tap creates a pass-through stream that calls fn for each signal.
func Tap(r *protocol.StreamReader[core.Signal], fn func(core.Signal)) *protocol.StreamReader[core.Signal] {
	return protocol.MapReader(r, func(s core.Signal) (core.Signal, error) {
		fn(s)
		return s, nil
	})
}

// FilterKind returns a stream that only passes signals of the given kinds.
func FilterKind(r *protocol.StreamReader[core.Signal], kinds ...core.SignalKind) *protocol.StreamReader[core.Signal] {
	set := make(map[core.SignalKind]struct{}, len(kinds))
	for _, k := range kinds {
		set[k] = struct{}{}
	}
	out, w := protocol.Pipe[core.Signal](0)
	go func() {
		defer w.Finish(nil)
		for {
			s, err := r.Recv()
			if err != nil {
				return
			}
			if _, ok := set[s.Kind]; ok {
				if err := w.Send(s); err != nil {
					return
				}
			}
		}
	}()
	return out
}

// CollectArtifacts extracts all artifact details from a signal slice.
func CollectArtifacts(signals []core.Signal) []core.ArtifactDetail {
	var out []core.ArtifactDetail
	for _, s := range signals {
		if s.Kind == core.SignalArtifact && s.Artifact != nil {
			out = append(out, *s.Artifact)
		}
	}
	return out
}
