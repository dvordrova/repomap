package storefixture

// A generic type's parameters are part of what it is; its fields and their
// struct tags are declarations of their own.

// Page is one page of any listed item.
type Page[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next,omitempty"`
}

// Tally keeps counters under a comparable key.
type Tally[K comparable, V map[string]int] struct {
	Entries map[K]V `json:"entries"`
}

// Labeled accepts only integer or string labels.
type Labeled[T interface{ ~int | ~string }] interface {
	Label() T
}

// Batch is constrained by another generic declaration of this package.
type Batch[T Labeled[int]] []T

// First returns the first item of any slice.
func First[T any](items []T) T { return items[0] }

// Keyed accepts any struct whose one field is tagged; the tag text holds a
// closing bracket that does not end the parameter list.
type Keyed[T interface {
	~struct {
		Key string `split:"]"`
	}
}] struct {
	Value T
}
