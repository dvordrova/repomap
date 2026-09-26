package clojureproject

import (
	"slices"
	"sort"
	"testing"
)

// A ; inside a string or a \; character literal is code; a definition's
// docstring is not, while a string argument elsewhere is, and so is the
// closing parenthesis on a docstring's line.
func TestCodeLinesSkipCommentsAndDocstrings(t *testing.T) {
	source := newSource([]byte(`(ns demo.core
  "The namespace docstring.")

;; a comment
(defn f
  "A docstring
  over two lines."
  [x]

  (str x ";" \;)) ; trailing
(def g "not a docstring")
`))
	var lines []int
	for line := range source.codeLines() {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	if want := []int{1, 2, 5, 8, 10, 11}; !slices.Equal(lines, want) {
		t.Fatalf("code lines %v, want %v", lines, want)
	}
}
