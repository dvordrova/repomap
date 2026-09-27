package terminology

import (
	"strings"
	"testing"
	"time"
)

// The glossary said event loop appeared in lzf.h and solarisfixes.h, which
// never write it: the list was the analysis context, not the term's places.
func TestOccurrencesListOnlyTheLinesThatWriteTheTerm(t *testing.T) {
	files := map[string]string{
		"ae.c":           "/* A simple event-driven programming library.\n * The event loop runs here.\n */\nint aeMain(void) { /* Event Loops */ }\n",
		"lzf.h":          "/* LZF compression */\n#define LZF_VERSION 0x0105\n",
		"solarisfixes.h": "#ifdef __sun\n#include <math.h>\n#endif\n",
		"redis.c":        "/* eventloop is not the phrase */\nstatic void beforeSleep(struct aeEventLoop *eventLoop) {}\n",
	}
	catalog := Catalog{Entries: []Entry{{ID: "d1", Names: []string{"event loop"}}, {ID: "d2", Names: []string{"LZF", "Lempel-Ziv"}}}}
	read := func(path string) (string, bool) { text, ok := files[path]; return text, ok }
	got := Occurrences(catalog, []string{"solarisfixes.h", "redis.c", "lzf.h", "ae.c", "missing.c"}, read)
	want := []TermOccurrences{
		{Entry: "d1", Name: "event loop", Sources: []Source{{Path: "ae.c", Line: 2}, {Path: "ae.c", Line: 4}}},
		{Entry: "d2", Name: "LZF", Sources: []Source{{Path: "lzf.h", Line: 1}}},
	}
	if len(got) != len(want) {
		t.Fatalf("occurrences %+v", got)
	}
	for i := range want {
		if got[i].Entry != want[i].Entry || got[i].Name != want[i].Name || len(got[i].Sources) != len(want[i].Sources) {
			t.Fatalf("occurrence %d: got %+v want %+v", i, got[i], want[i])
		}
		for j := range want[i].Sources {
			if got[i].Sources[j] != want[i].Sources[j] {
				t.Fatalf("occurrence %d: got %+v want %+v", i, got[i], want[i])
			}
		}
	}
}

// Every hit counted the lines from the start of its file again: a large
// generated file writing a term on every line took quadratic time in the
// ordinary run.
func TestOccurrencesCountEachLineOnceInALargeFile(t *testing.T) {
	var text strings.Builder
	for i := 0; i < 250_000; i++ {
		text.WriteString("the request is here\n")
	}
	body := text.String()
	catalog := Catalog{Entries: []Entry{{ID: "d1", Names: []string{"request"}}}}
	started := time.Now()
	got := Occurrences(catalog, []string{"generated.go"}, func(string) (string, bool) { return body, true })
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("one 5 MB file took %s", elapsed)
	}
	if len(got) != 1 || len(got[0].Sources) != 250_000 || got[0].Sources[249_999].Line != 250_000 || got[0].Sources[1].Line != 2 {
		t.Fatalf("lines: %d", len(got[0].Sources))
	}
}
