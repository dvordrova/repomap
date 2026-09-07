//go:build repomap_optional_tests

package cumulativegofixture

import "testing"

func TestOptionalRoot(t *testing.T) {
	if PublishedRoot() != testExpectedRoot() {
		t.Fatal("unexpected root")
	}
}
