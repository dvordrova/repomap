package partstest

import (
	"slices"
	"testing"
)

// The preset divides a box by the next path segment under the directory its
// files share, so the grouping ends at one box per file.
func TestNextSegmentsEndAtOneFile(t *testing.T) {
	for _, tc := range []struct {
		paths, want []string
	}{
		{[]string{"a/b/x.go", "a/b/y.go"}, []string{"a/b/x.go", "a/b/y.go"}},
		{[]string{"a/b/x.go", "a/c/y.go", "a/c/z.go"}, []string{"a/b", "a/c"}},
		{[]string{"kvd.c", "pkg/a.go"}, []string{"kvd.c", "pkg"}},
		{[]string{"a/x.go", "a/b/y.go"}, []string{"a/x.go", "a/b"}},
		{[]string{"a/b/x.go"}, []string{"a/b/x.go"}},
	} {
		if got := nextSegments(tc.paths); !slices.Equal(got, tc.want) {
			t.Errorf("nextSegments(%v) = %v, want %v", tc.paths, got, tc.want)
		}
	}
}
