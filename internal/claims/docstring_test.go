package claims

import (
	"strings"
	"testing"
)

func TestPythonDocstringsUseFirstStatementOnly(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		`#!/usr/bin/env python`,
		`# comment before the docstring`,
		`'''Module doc.'''`,
		`import os`,
		``,
		`class Game:`,
		`    """A game."""`,
		`    def play(self):`,
		`        """Nested methods are not top-level."""`,
		``,
		`async def run(`,
		`    arg,`,
		`):  # trailing comment`,
		`    x = 1`,
		`    """Not a docstring: not the first statement."""`,
		``,
		`def load(path):`,
		`    r"""Load a file.`,
		``,
		`    Second paragraph."""`,
	}, "\n"))
	quotes := pythonDocstrings(lines)
	want := []quote{
		{Line: 3, Text: "Module doc."},
		{Line: 7, Text: "A game."},
		{Line: 18, Text: "Load a file. Second paragraph."},
	}
	if len(quotes) != len(want) {
		t.Fatalf("quotes = %+v, want %+v", quotes, want)
	}
	for index := range want {
		if quotes[index] != want[index] {
			t.Fatalf("quote %d = %+v, want %+v", index, quotes[index], want[index])
		}
	}
}

func TestGoDocCommentsSkipDirectivesAndDetachedComments(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		`package x`,
		``,
		`// detached comment`,
		``,
		`//go:generate go run gen.go`,
		`// Config holds settings.`,
		`type Config struct{}`,
		``,
		`func undocumented() {}`,
		``,
		`// Run starts the server.`,
		`// It blocks.`,
		`func Run() {}`,
	}, "\n"))
	quotes := goDocComments(lines)
	want := []quote{{Line: 5, Text: "Config holds settings."}, {Line: 11, Text: "Run starts the server. It blocks."}}
	if len(quotes) != len(want) || quotes[0] != want[0] || quotes[1] != want[1] {
		t.Fatalf("quotes = %+v, want %+v", quotes, want)
	}
}

func TestJSDocBlocksRequireDeclarationAndDropTags(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		`/** Not attached to a declaration. */`,
		``,
		`const x = 1;`,
		`/** One-liner. */`,
		`export const y = 2;`,
		`/**`,
		` * Loads a level.`,
		` * @param id the level id`,
		` */`,
		`export default class Loader {}`,
		`/* plain block */`,
		`function z() {}`,
	}, "\n"))
	quotes := jsDocBlocks(lines)
	want := []quote{{Line: 4, Text: "One-liner."}, {Line: 6, Text: "Loads a level."}}
	if len(quotes) != len(want) || quotes[0] != want[0] || quotes[1] != want[1] {
		t.Fatalf("quotes = %+v, want %+v", quotes, want)
	}
}

func TestMarkerCommentsInspectOnlyCommentText(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		`x = "NOTE: inside a string"`,
		`y = 1  # important: lowercase marker`,
		`# TODO: facts, not claims`,
		`/* WARNING: block start */`,
		` * This API is deprecated since 2.0`,
		`// keep DEPRECATED_FLAG name`,
	}, "\n"))
	python := markerComments(lines, "#")
	if len(python) != 1 || python[0] != (quote{Line: 2, Text: "important: lowercase marker"}) {
		t.Fatalf("python quotes = %+v", python)
	}
	js := markerComments(lines, "//")
	want := []quote{{Line: 4, Text: "WARNING: block start"}, {Line: 5, Text: "This API is deprecated since 2.0"}}
	if len(js) != len(want) || js[0] != want[0] || js[1] != want[1] {
		t.Fatalf("js quotes = %+v, want %+v", js, want)
	}
}

func TestQuoteWithinPrefersParagraphThenSentence(t *testing.T) {
	long := strings.Repeat("x", MaxTextRunes)
	if got := quoteWithin("First para.\n\n" + long); got != "First para." {
		t.Fatalf("paragraph fallback = %q", got)
	}
	if got := quoteWithin("First sentence. " + long); got != "First sentence." {
		t.Fatalf("sentence fallback = %q", got)
	}
	if got := quoteWithin(long + " tail"); !fits(got) || got != long {
		t.Fatalf("word cut = %d runes", len(got))
	}
}
