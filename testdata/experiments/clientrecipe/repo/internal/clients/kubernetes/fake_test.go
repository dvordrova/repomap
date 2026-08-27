package kubernetes

import "context"

// fakeNamespaceLister demonstrates a fake kept in a separate test-only file.
type fakeNamespaceLister struct{}

func (fakeNamespaceLister) ListNamespaces(context.Context) ([]string, error) {
	return []string{"fake"}, nil
}
