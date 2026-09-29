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

// SQL a test runs is the test's, not the library's: its table reaches no data
// and its call no outbound call of the program.
func createTestOnlyRows(execute func(string) error) error {
	return execute("CREATE TABLE test_only_rows (id INTEGER PRIMARY KEY)")
}
