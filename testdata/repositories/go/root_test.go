package cumulativegofixture

import "testing"

func TestPublishedRoot(t *testing.T) {
	if got := PublishedRoot(); got != "repository root" {
		t.Fatalf("PublishedRoot = %q", got)
	}
}

func testExpectedRoot() string { return "repository root" }

// A method in a test can have a receiver declared in ordinary source.
func (*testRootReader) expected() string { return "repository root" }

// A method can precede its test-local receiver declaration.
func (*testObserver) observe() string { return "observed" }

type testObserver struct{}
