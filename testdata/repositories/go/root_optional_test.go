//go:build repomap_optional_tests

package cumulativegofixture

import "testing"

func TestOptionalRoot(t *testing.T) {
	if PublishedRoot() != testExpectedRoot() {
		t.Fatal("unexpected root")
	}
}

// SQL a test only another build selects is no program's: no load holds this
// file, so its table reaches no data.
func createOptionalRows(execute func(string) error) error {
	return execute("CREATE TABLE test_only_rows (id INTEGER PRIMARY KEY)")
}
