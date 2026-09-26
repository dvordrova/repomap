package places

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
)

// A C file's author comments reach its declarations through the ordinary
// claims: the comment above a function is that function's description, a
// comment inside a body is nobody's, and the file is described by its
// author's opening comment, not by the licence above it (D5) or a section
// banner (D4).
func TestCCommentsDescribeTheirOwnDeclarations(t *testing.T) {
	const source = `/* Copyright (c) 2009, An Author <author at example dot com>
 * All rights reserved.
 */

/* kv.c -- a key-value store kept in memory. */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "kv.h"

#define KV_BUCKETS 16

/* ================================ Lookup ================================ */
static int kvBuckets = KV_BUCKETS;

/* Return the value stored under key. */
char *kvGet(const char *key) {
    /* Scan every bucket. */
    return kvScan(key, kvBuckets);
}
int kvSize(void) {
    return kvBuckets;
}

/* Scan the buckets for key. */
char *kvScan(const char *key, int buckets);
struct kvEntry {
    char *key;
};

/* Count the stored keys. */
static int
kvCount(void)
{
    return 0;
}

/* One stored pair. */
struct kvPair
{
    char *key;
};
`
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "kv.c"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	root, repository := materializeFixtureRepository(t, fixture)
	quoted, err := claims.Extract(t.Context(), claims.Input{Repository: repository, RepoPath: root, Revision: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	b := builder{docs: map[string][]claims.Claim{}}
	for _, claim := range quoted.Claims {
		if claim.Source == claims.SourceDocstring {
			b.docs[claim.Path] = append(b.docs[claim.Path], claim)
		}
	}
	line := func(needle string) int {
		t.Helper()
		return strings.Count(source[:strings.Index(source, needle)], "\n") + 1
	}
	// The index holds definitions, located at their names; the prototype of
	// kvScan is no declaration of this file.
	decls := []atlas.Decl{{LineNo: line("static int kvBuckets")}, {LineNo: line("char *kvGet")}, {LineNo: line("int kvSize")},
		{LineNo: line("struct kvEntry")}, {LineNo: line("kvCount(void)")}, {LineNo: line("struct kvPair")}}
	for _, want := range []struct {
		decl int
		doc  string
	}{{0, ""}, {1, "Return the value stored under key."}, {2, ""}, {3, ""}, {4, "Count the stored keys."}, {5, "One stored pair."}} {
		if got := b.docstringFor("kv.c", decls[want.decl].LineNo, decls); got != want.doc {
			t.Errorf("declaration at line %d: %q, want %q", decls[want.decl].LineNo, got, want.doc)
		}
	}
	if got := b.moduleDoc("kv.c", &fileState{decls: decls}); got != "kv.c -- a key-value store kept in memory." {
		t.Errorf("file description %q", got)
	}
}
