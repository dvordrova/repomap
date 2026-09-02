package cumulativegofixture

// PublishedRoot is mirrored by an external version of this module used by the
// nested-module fixture. The shared import path must not make that external
// version part of the repository-local package closure.
func PublishedRoot() string {
	return "repository root"
}
