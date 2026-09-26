package cproject

import (
	"slices"
	"sort"
	"testing"
)

// A line holds code when a character outside a comment is not blank. The
// lexer must not read comment markers inside literals, and a // comment
// continued by a backslash-newline swallows the next line.
func TestCodeLinesSkipCommentsNotLiterals(t *testing.T) {
	source := []byte(`/* header
 * still the header */
#define TWICE(x) \
    ((x) * 2)

int f(void) {
    // a comment \
       continued onto this line
    const char *s = "not // a comment";
    char c = '/';
    /* a block */ return TWICE(c); /* trailing
    */
}
`)
	var lines []int
	for line := range codeLines(source) {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	if want := []int{3, 4, 6, 9, 10, 11, 13}; !slices.Equal(lines, want) {
		t.Fatalf("code lines %v, want %v", lines, want)
	}
}
