package claims

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestShortReadmeIsOneQuoteAtLineOne(t *testing.T) {
	text := "# Title\n\nFirst   paragraph\nspans lines.\n\n```\ncode is skipped\n```\n\nSecond paragraph.\n"
	quotes := readmeQuotes(len(text), splitLines(text))
	if len(quotes) != 1 || quotes[0].Line != 1 {
		t.Fatalf("quotes = %+v, want one quote at line 1", quotes)
	}
	// Formatting markers are dropped so the quote reads as the sentence the
	// author wrote.
	if quotes[0].Text != "Title First paragraph spans lines. Second paragraph." {
		t.Fatalf("text = %q", quotes[0].Text)
	}
}

func TestShortReadmeSplitsAtParagraphsWhenTooLong(t *testing.T) {
	// Each paragraph fits on its own; only the whole text exceeds the bound.
	long := strings.Repeat("word ", MaxTextRunes/6)
	text := "Intro line.\n\n" + long + "\n\n" + long + "\nend.\n"
	if len(text) > MaxWholeReadmeBytes {
		t.Fatalf("fixture must stay a short README, got %d bytes", len(text))
	}
	quotes := readmeQuotes(len(text), splitLines(text))
	if len(quotes) != 3 {
		t.Fatalf("quotes = %d, want one per paragraph: %+v", len(quotes), quotes)
	}
	if quotes[0].Line != 1 || quotes[1].Line != 3 || quotes[2].Line != 5 {
		t.Fatalf("lines = %d %d %d", quotes[0].Line, quotes[1].Line, quotes[2].Line)
	}
	if quotes[0].Text != "Intro line." || !strings.HasSuffix(quotes[2].Text, "word end.") {
		t.Fatalf("texts = %q ... %q", quotes[0].Text, quotes[2].Text)
	}
}

func TestOversizedParagraphIsSplitWithoutLosingWords(t *testing.T) {
	words := make([]string, 0, MaxTextRunes)
	for index := 0; index < MaxTextRunes; index++ {
		words = append(words, "w")
	}
	pieces := splitWithin(strings.Join(words, " "))
	if len(pieces) < 2 {
		t.Fatalf("pieces = %d, want a split", len(pieces))
	}
	total := 0
	for _, piece := range pieces {
		if !fits(piece) {
			t.Fatalf("piece of %d runes exceeds the bound", utf8.RuneCountInString(piece))
		}
		total += len(strings.Fields(piece))
	}
	if total != len(words) {
		t.Fatalf("words after split = %d, want %d", total, len(words))
	}
}

func TestLongReadmeQuotesHeadingsWithFirstParagraph(t *testing.T) {
	filler := strings.Repeat("filler text that pushes the file over the whole-quote threshold. ", 40)
	text := strings.Join([]string{
		"# Tutorial Game",
		"",
		"A game for learning.",
		"Two lines long.",
		"",
		"Second paragraph is not quoted.",
		"",
		"## Install",
		"```",
		"pip install x",
		"```",
		"Run the installer.",
		"",
		"## Empty ##",
		"",
		"### Details",
		filler,
		"",
	}, "\n")
	if len(text) <= MaxWholeReadmeBytes {
		t.Fatalf("fixture must exceed the whole-quote threshold, got %d bytes", len(text))
	}
	quotes := readmeQuotes(len(text), splitLines(text))
	// A heading with no prose beneath it ("## Empty ##") states nothing and is
	// skipped; the next heading with a body follows it directly.
	want := []quote{
		{Line: 1, Text: "Tutorial Game — A game for learning. Two lines long."},
		{Line: 8, Text: "Install — Run the installer."},
	}
	if len(quotes) < len(want) {
		t.Fatalf("quotes = %+v", quotes)
	}
	for index, expected := range want {
		if quotes[index] != expected {
			t.Fatalf("quote %d = %+v, want %+v", index, quotes[index], expected)
		}
	}
	var pieces []string
	for _, rest := range quotes[len(want):] {
		if rest.Line != 16 || !fits(rest.Text) {
			t.Fatalf("details quote %+v is not a bounded piece at the heading line", rest)
		}
		pieces = append(pieces, rest.Text)
	}
	if joined := strings.Join(pieces, " "); joined != "Details — "+collapseSpace(filler) {
		t.Fatalf("details pieces do not rebuild the heading paragraph: %q", joined)
	}
}

func TestLongReadmeWithoutHeadingsQuotesFirstParagraph(t *testing.T) {
	text := "\n\nOpening paragraph.\n\n" + strings.Repeat("more text. ", 300)
	quotes := readmeQuotes(len(text), splitLines(text))
	if len(quotes) != 1 || quotes[0].Line != 3 || quotes[0].Text != "Opening paragraph." {
		t.Fatalf("quotes = %+v", quotes)
	}
}

func TestReadmeNameMatching(t *testing.T) {
	for _, filePath := range []string{"README", "readme.md", "docs/README.rst", "Readme.txt"} {
		if !isReadmePath(filePath) {
			t.Fatalf("%s should be a README", filePath)
		}
	}
	for _, filePath := range []string{"readme_old.md", "READMEs.md", "notes.md"} {
		if isReadmePath(filePath) {
			t.Fatalf("%s should not be a README", filePath)
		}
	}
}

func TestReadmeQuotesProseRatherThanMarkup(t *testing.T) {
	text := strings.Join([]string{
		`<img alt="chi" src="https://cdn.example/chi.svg" width="220" />`,
		"",
		"[![GoDoc Widget]][GoDoc] [![Go Report Card]][GoReportCard]",
		"",
		"`chi` is a **lightweight**, composable router for building Go services.",
		"See the [docs](https://example.test/docs) for details.",
		"",
	}, "\n")
	quotes := readmeQuotes(len(text), splitLines(text))
	if len(quotes) != 1 {
		t.Fatalf("quotes = %+v", quotes)
	}
	want := "chi is a lightweight, composable router for building Go services. " +
		"See the docs for details."
	if quotes[0].Text != want {
		t.Fatalf("quote = %q, want %q", quotes[0].Text, want)
	}
}

func TestReadableDropsMarkupOnlyFragments(t *testing.T) {
	for _, markup := range []string{
		`<img src="x.svg" />`,
		"[![Badge]][link]",
		"![shot](shot.png)",
		"---",
		"**",
	} {
		if got := readable(markup); got != "" {
			t.Fatalf("readable(%q) = %q, want no prose", markup, got)
		}
	}
}
