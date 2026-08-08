package tasklens

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitLinesHandlesHugeAndTrailingNewline(t *testing.T) {
	t.Parallel()
	// One multi-megabyte line (statik-style) must split into exactly one
	// line — no scanner token ceiling.
	huge := strings.Repeat("x", 2<<20)
	lines := splitLines([]byte(huge))
	if len(lines) != 1 || lines[0] != huge {
		t.Fatalf("splitLines(huge) = %d lines", len(lines))
	}
	// Normal file with trailing newline: no phantom empty last line.
	got := splitLines([]byte("a\nb\n"))
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("splitLines(a\\nb\\n) = %#v", got)
	}
	// Empty input.
	if got := splitLines(nil); got != nil {
		t.Fatalf("splitLines(nil) = %#v, want nil", got)
	}
	// Empty lines in the middle are preserved (git grep blank hits).
	got = splitLines([]byte("a\n\nb"))
	if !reflect.DeepEqual(got, []string{"a", "", "b"}) {
		t.Fatalf("splitLines(a\\n\\nb) = %#v", got)
	}
}
