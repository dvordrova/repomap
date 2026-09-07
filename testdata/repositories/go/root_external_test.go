package cumulativegofixture_test

import (
	"testing"

	fixture "example.com/repomap/cumulative-go-fixture"
)

func TestPublishedAPI(t *testing.T) {
	if got := fixture.PublishedRoot(); got != "repository root" {
		t.Fatalf("PublishedRoot = %q", got)
	}
}
