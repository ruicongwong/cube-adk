package option

// Option is a two-layer functional option. The applyFn modifies the common
// options struct T, while implFn carries an implementation-specific function
// that can be extracted by the concrete implementation.
type Option[T any] struct {
	applyFn func(*T)
	implFn  any
}

// NewOption creates an option that modifies the common options struct.
func NewOption[T any](fn func(*T)) Option[T] {
	return Option[T]{applyFn: fn}
}

// WrapImpl creates an option that carries an implementation-specific function.
// The common applyFn is nil; only the implFn is set.
func WrapImpl[T any, I any](fn func(*I)) Option[T] {
	return Option[T]{implFn: fn}
}

// Apply applies all options to the base struct in order.
func Apply[T any](base *T, opts ...Option[T]) {
	for _, o := range opts {
		if o.applyFn != nil {
			o.applyFn(base)
		}
	}
}

// ExtractImpl collects all implementation-specific option functions of type I.
func ExtractImpl[T any, I any](opts ...Option[T]) []func(*I) {
	var fns []func(*I)
	for _, o := range opts {
		if o.implFn == nil {
			continue
		}
		if fn, ok := o.implFn.(func(*I)); ok {
			fns = append(fns, fn)
		}
	}
	return fns
}
