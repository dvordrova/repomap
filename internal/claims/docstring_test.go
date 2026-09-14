package claims

import (
	"os"
	"path/filepath"
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
		`        """Play the game."""`,
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
		{Line: 7, DeclarationLine: 6, Text: "A game."},
		{Line: 9, DeclarationLine: 8, Text: "Play the game."},
		{Line: 18, DeclarationLine: 17, Text: "Load a file. Second paragraph."},
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

func TestCumulativePythonDocstringsKeepNestedDeclarationOwners(t *testing.T) {
	filePath := "src/fixture_app/destinations.py"
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "python", filePath))
	if err != nil {
		t.Fatal(err)
	}
	lines := splitLines(string(data))
	byText := make(map[string]quote)
	for _, item := range pythonDocstrings(lines) {
		byText[item.Text] = item
	}
	for _, want := range []struct{ declaration, text string }{
		{"class DocumentedClient:", "An author-described client used by a local test."},
		{"    def setup(self):", "Build the client with a nested test helper."},
		{"        def setup_mock_client():", "Set up a mock HTTP client for the test."},
		{"    async def ready(self):", "Report that the test setup is ready."},
	} {
		line := 0
		for i, text := range lines {
			if text == want.declaration {
				line = i + 1
				break
			}
		}
		got := byText[want.text]
		if line == 0 || got.DeclarationLine != line || got.Line != line+1 {
			t.Errorf("%s: owner/quote location = %+v, declaration line %d", want.declaration, got, line)
		}
	}
	if _, exists := byText["A later string is not documentation for this method."]; exists {
		t.Fatal("later expression became an author docstring")
	}
}

func TestPythonDocstringsDoNotTreatStringContentsAsDeclarations(t *testing.T) {
	lines := splitLines(strings.Join([]string{
		`example = """`,
		`class Example:`,
		`    '''An example inside a string.'''`,
		`"""`,
		`def outer():`,
		`    """Outer author quote."""`,
		`    async def nested(`,
		`        value="hash # and colon:",`,
		`    ):`,
		`        # A comment before the actual first statement.`,
		`        r'''Nested author quote.'''`,
		`        return value`,
		`    return nested`,
		`def bytes_only():`,
		`    b"""Bytes are not a docstring."""`,
		`def interpolation_only():`,
		`    f"""An interpolated expression is not a docstring."""`,
		`def later():`,
		`    pass`,
		`"""A dedented later string belongs to no declaration."""`,
		`def inline(): pass`,
		`class Inline: """An inline string is not a following body."""`,
		`def following():`,
		`    """The following declaration owns this quote."""`,
	}, "\n"))
	want := []quote{
		{Line: 6, DeclarationLine: 5, Text: "Outer author quote."},
		{Line: 11, DeclarationLine: 7, Text: "Nested author quote."},
		{Line: 24, DeclarationLine: 23, Text: "The following declaration owns this quote."},
	}
	got := pythonDocstrings(lines)
	if len(got) != len(want) {
		t.Fatalf("quotes = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quote %d = %+v, want %+v", i, got[i], want[i])
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

func TestGoInterfaceContractsKeepAttachedMethodDocumentation(t *testing.T) {
	quotes := goDocComments(splitLines(`package x
type Ticket[T any] interface {
 // Cancel revokes a ticket. Pending jobs are removed from the queue.
 Cancel(id T) error
 // Detached comment must not describe Status.

 Status(id T) string
 // Embedded documentation is not a method declaration.
 Other
}
type Other interface{}
func local() {
 type Nested interface {
  // Incidental local interface, outside the native declaration scope.
  Run()
 }
}
var anonymous interface {
 // Anonymous variable interface, not a named contract.
 Run()
}
`))
	if len(quotes) != 1 || quotes[0].Line != 3 || quotes[0].Text != "Cancel revokes a ticket. Pending jobs are removed from the queue." {
		t.Fatalf("interface documentation lost or misattached: %+v", quotes)
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
