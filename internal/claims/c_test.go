package claims

import (
	"reflect"
	"strings"
	"testing"
)

// cSource is a small C file with the comment shapes real C code has: a
// licence at the top, the author's description, section banners, comments
// above declarations and comments inside bodies.
const cSource = `/* Copyright (c) 2009, An Author <author at example dot com>
 * All rights reserved.
 *
 * NOTE: Redistribution and use in source and binary forms are permitted.
 */

/*	$Id: kv.c,v 1.2 2009/01/30 12:00:00 author Exp $	*/

/* kv.c -- a small key-value store kept in memory. */

#include <stdio.h>
#include "kv.h"

/* ================================ Globals ================================= */
static int kvReady;

/* The table every command reads. */
static struct table *kvTable;

/*-----------------------------------------------------------------------------
 * Commands
 *----------------------------------------------------------------------------*/

// Look a command up by the name the client sent.
// Unknown names return NULL.
static struct command *lookupCommand(const char *name) {
    /* NOTE: the table is sorted, but this scan does not rely on it. */
    for (int i = 0; tableSize(kvTable) > i; i++) {
        /* Compare the whole name. */
        if (!strcmp(kvTable->names[i], name)) return kvTable->commands[i];
    }
    return NULL;
}

/* Kept for the old protocol. */

int protocolVersion = 1;

/* Two keys per bucket. */
#define BUCKET_KEYS 2

/* Print a greeting. */ int greet(void);

/* Reply with the value.  WARNING: the caller frees it. */
int getCommand(struct client *c);

const char *banner = "/* NOTE: not a comment */ // WARNING: nor this";

/* Start the server.
 *
 * Reads the configuration first. */
int
main(int argc, char **argv)
{
    int x = 1; /* IMPORTANT: counted from one. */
    return x;
}
`

func TestCQuotes(t *testing.T) {
	docs, markers := cQuotes(splitLines(cSource))
	line := func(needle string) int {
		t.Helper()
		at := strings.Index(cSource, needle)
		if at < 0 {
			t.Fatalf("%q not in the source", needle)
		}
		return strings.Count(cSource[:at], "\n") + 1
	}
	wantDocs := []quote{
		{Line: line("/* kv.c --"), Text: "kv.c -- a small key-value store kept in memory."},
		{Line: line("/* The table every"), DeclarationLine: line("static struct table"), Text: "The table every command reads."},
		{Line: line("// Look a command"), DeclarationLine: line("static struct command"), Text: "Look a command up by the name the client sent. Unknown names return NULL."},
		{Line: line("/* Reply with"), DeclarationLine: line("int getCommand"), Text: "Reply with the value. WARNING: the caller frees it."},
		{Line: line("/* Start the server."), DeclarationLine: line("main(int argc"), Text: "Start the server. Reads the configuration first."},
	}
	if !reflect.DeepEqual(docs, wantDocs) {
		t.Fatalf("docstrings:\n%+v\nwant\n%+v", docs, wantDocs)
	}
	wantMarkers := []quote{
		{Line: line("/* NOTE: the table"), Text: "NOTE: the table is sorted, but this scan does not rely on it."},
		{Line: line("/* Reply with"), Text: "Reply with the value. WARNING: the caller frees it."},
		{Line: line("/* IMPORTANT:"), Text: "IMPORTANT: counted from one."},
	}
	if !reflect.DeepEqual(markers, wantMarkers) {
		t.Fatalf("markers:\n%+v\nwant\n%+v", markers, wantMarkers)
	}
}

// A licence that is not at the top of the file is an ordinary comment, and a
// file that starts with code has no leading description.
func TestCQuotesKeepLaterLicenceComments(t *testing.T) {
	docs, _ := cQuotes(splitLines("#include <stdio.h>\n/* Released under the MIT license. */\nint licensed;\n"))
	if len(docs) != 1 || docs[0].Line != 2 || docs[0].DeclarationLine != 3 || docs[0].Text != "Released under the MIT license." {
		t.Fatalf("docs %+v", docs)
	}
	docs, markers := cQuotes(splitLines("/* SPDX-License-Identifier: MIT */\n// NOTE: generated\nint x;\n"))
	if len(docs) != 1 || docs[0].Text != "NOTE: generated" || len(markers) != 1 || markers[0].Line != 2 {
		t.Fatalf("docs %+v markers %+v", docs, markers)
	}
}

func TestCFilesAreQuoted(t *testing.T) {
	for _, path := range []string{"src/kv.c", "include/kv.h", "KV.H"} {
		if classifyPath(path) != kindC {
			t.Fatalf("%s is not read as C", path)
		}
	}
}
