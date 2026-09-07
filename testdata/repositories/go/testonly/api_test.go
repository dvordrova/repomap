package testonly_test

import (
	"testing"

	fixture "example.com/repomap/cumulative-go-fixture"
)

func TestRootFromTestOnlyPackage(t *testing.T) {
	if fixture.PublishedRoot() == "" {
		t.Fatal("empty root")
	}
}
