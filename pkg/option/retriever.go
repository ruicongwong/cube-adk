package option

// RetrieverOpts holds common options for retrieval operations.
type RetrieverOpts struct {
	TopK           int
	ScoreThreshold float64
}

// RetrieverOption is a functional option for retrieval.
type RetrieverOption = Option[RetrieverOpts]

// WithTopK sets the maximum number of documents to retrieve.
func WithTopK(k int) RetrieverOption {
	return NewOption[RetrieverOpts](func(o *RetrieverOpts) { o.TopK = k })
}

// WithScoreThreshold sets the minimum relevance score.
func WithScoreThreshold(t float64) RetrieverOption {
	return NewOption[RetrieverOpts](func(o *RetrieverOpts) { o.ScoreThreshold = t })
}
