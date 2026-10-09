package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/claims"
)

// A C symbol's card quotes only the comment written on its own declaration.
// The page reads the same claims places does, so the file's description and
// a comment above a prototype are no symbol's, though both sit within reach
// above the next one:
//
//	 1  /* kv.c -- a key-value store kept in memory. */
//	 2
//	 3  #include "kv.h"
//	 4
//	 5  static int kvBuckets = 16;
//	 6
//	 7  /* Scan the buckets for key. */
//	 8  char *kvScan(const char *key);
//	 9  struct kvEntry {
//	10      char *key;
//	11  };
//	12
//	13  /* Count the stored keys. */
//	14  static int
//	15  kvCount(void)
//	16  {
func TestCDocstringsDescribeOnlyTheirOwnDeclaration(t *testing.T) {
	builder := pageBuilder{
		docstrings: map[string][]claims.Claim{"kv.c": {
			{Source: claims.SourceDocstring, Path: "kv.c", Line: 1, Text: "kv.c -- a key-value store kept in memory."},
			{Source: claims.SourceDocstring, Path: "kv.c", Line: 7, DeclarationLine: 8, Text: "Scan the buckets for key."},
			{Source: claims.SourceDocstring, Path: "kv.c", Line: 13, DeclarationLine: 15, Text: "Count the stored keys."},
		}},
		declarations: map[string][]int{"kv.c": {5, 9, 15}},
	}
	for line, want := range map[int]string{5: "", 9: "", 15: "Count the stored keys."} {
		if got := builder.docstringFor("kv.c", line); got != want {
			t.Errorf("declaration at line %d: %q, want %q", line, got, want)
		}
	}
}

func TestNativeCommentCardsUseExactSourceTuple(t *testing.T) {
	for _, source := range []string{"native.c", "native.clj"} {
		builder := pageBuilder{docstrings: map[string][]claims.Claim{source: {{Source: claims.SourceDocstring, Path: source, Line: 1, DeclarationLine: 2, DeclarationColumn: 5, Text: "The first declaration owns this description."}}}, declarations: map[string][]int{source: {2, 2}}}
		if builder.docstringFor(source, 2, 5) == "" || builder.docstringFor(source, 2, 35) != "" {
			t.Fatalf("same-line cards leaked owner for %s", source)
		}
	}
}
