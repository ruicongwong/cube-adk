package runtime

import "cube-adk/pkg/core"

// Collect drains a signal channel into a slice.
func Collect(ch <-chan core.Signal) []core.Signal {
	var out []core.Signal
	for s := range ch {
		out = append(out, s)
	}
	return out
}

// Tap creates a pass-through channel that calls fn for each signal.
func Tap(ch <-chan core.Signal, fn func(core.Signal)) <-chan core.Signal {
	out := make(chan core.Signal)
	go func() {
		defer close(out)
		for s := range ch {
			fn(s)
			out <- s
		}
	}()
	return out
}

// FilterKind returns a channel that only passes signals of the given kinds.
func FilterKind(ch <-chan core.Signal, kinds ...core.SignalKind) <-chan core.Signal {
	set := make(map[core.SignalKind]struct{}, len(kinds))
	for _, k := range kinds {
		set[k] = struct{}{}
	}
	out := make(chan core.Signal)
	go func() {
		defer close(out)
		for s := range ch {
			if _, ok := set[s.Kind]; ok {
				out <- s
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
