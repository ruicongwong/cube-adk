package component

// ComponentKind identifies the type of a component.
type ComponentKind int

const (
	KindModel     ComponentKind = iota
	KindTool
	KindRetriever
	KindEmbedder
)

func (k ComponentKind) String() string {
	switch k {
	case KindModel:
		return "model"
	case KindTool:
		return "tool"
	case KindRetriever:
		return "retriever"
	case KindEmbedder:
		return "embedder"
	default:
		return "unknown"
	}
}

// Typer identifies a concrete implementation type.
type Typer interface {
	GetType() string
}
