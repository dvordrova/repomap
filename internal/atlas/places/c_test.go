package places

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/cproject"
	"github.com/dvordrova/repomap/internal/programindex"
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
	if err := os.WriteFile(filepath.Join(fixture, "kv.c"), []byte(source+"\nchar *kvScan(const char *key, int buckets) { return NULL; }\nint main(void) { return kvCount(); }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "kv.h"), []byte("char *kvScan(const char *key, int buckets);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, repository := materializeFixtureRepository(t, fixture)
	project, err := cproject.Discover(t.Context(), root, repository)
	if err != nil || len(project.Programs) != 1 {
		t.Fatalf("real C fixture discovery: %v %v", project, err)
	}
	parsed, err := cproject.Parse(t.Context(), root, repository, project.Programs[0], cproject.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	result, err := cproject.Index(repository, parsed)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := claims.Extract(t.Context(), claims.Input{Repository: repository, RepoPath: root, Revision: "HEAD", ReadIndexes: []func() (programindex.Index, error){func() (programindex.Index, error) { return index, nil }}})
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
	for i := range decls {
		for _, object := range index.Objects {
			if object.Location != nil && object.Location.Path == "kv.c" && object.Location.Line == decls[i].LineNo {
				decls[i].Column = object.Location.Column
			}
		}
	}
	for _, want := range []struct {
		decl int
		doc  string
	}{{0, ""}, {1, "Return the value stored under key."}, {2, ""}, {3, ""}, {4, "Count the stored keys."}, {5, "One stored pair."}} {
		if got := b.docstringFor("kv.c", decls[want.decl].LineNo, decls, decls[want.decl].Column); got != want.doc {
			t.Errorf("declaration at line %d: %q, want %q", decls[want.decl].LineNo, got, want.doc)
		}
	}
	if got := b.moduleDoc("kv.c", &fileState{decls: decls}); got != "kv.c -- a key-value store kept in memory." {
		t.Errorf("file description %q", got)
	}
}
