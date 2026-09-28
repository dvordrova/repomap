package report

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/facts"
)

// The fact knows only that no recognized test file is among the inspected
// paths, and the page says no more than that in either language. "No test
// files found" read as "this repository has no tests": a newcomer writing
// only from the Redis report told readers to skip test-redis.tcl, 204 tests
// no adapter recognizes as tests.
func TestNoTestsNegativeSaysOnlyWhatIsRecognized(t *testing.T) {
	data := &ReportData{Facts: &facts.Result{Facts: []facts.Fact{
		{Kind: facts.KindNegative, Key: facts.NegativeNoTests, Text: "no recognized test files found in inspected paths"},
		{Kind: facts.KindNegative, Key: facts.NegativeNoCI, Text: "no recognized CI configuration found in inspected paths"},
	}}}
	builder := &pageBuilder{data: data, links: newPageLinks(data)}
	english := &pageView{}
	builder.negatives(english)
	russian := &pageView{}
	builder.negatives(russian)
	if err := (&PreparedPage{view: russian}).applyUI(Russian); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		view *pageView
		want []string
	}{
		{english, []string{
			"No recognized test files found in the inspected paths.",
			"No CI configuration found (no recognized CI configuration found in inspected paths).",
		}},
		{russian, []string{
			"В просмотренных путях не распознаны тестовые файлы.",
			"Настройки CI не найдены (в просмотренных путях не распознаны настройки CI).",
		}},
	} {
		if len(check.view.Negatives) != len(check.want) {
			t.Fatalf("negatives %+v, want %q", check.view.Negatives, check.want)
		}
		for i, want := range check.want {
			if got := check.view.Negatives[i].Text; got != want {
				t.Fatalf("negative %d = %q, want %q", i, got, want)
			}
		}
	}
}

// Code no adapter analyses is named beside the negatives, by language with
// its files: the most code first, the biggest file first, so "No recognized
// test files" stands next to test-redis.tcl's 2,083 lines.
func TestUnanalysedFilesStandByLanguageMostCodeFirst(t *testing.T) {
	file := func(path, language string, lines int) facts.Fact {
		return facts.Fact{Kind: facts.KindUnanalysedFile, Anchor: &facts.Anchor{Path: path}, Key: language, Lines: lines}
	}
	data := &ReportData{Facts: &facts.Result{Facts: []facts.Fact{
		file("redis.tcl", "Tcl", 131), file("test-redis.tcl", "Tcl", 2083), file("utils/build-static-symbols.tcl", "Tcl", 22),
		file("utils/redis-copy.rb", "Ruby", 80), file("utils/redis-sha1.rb", "Ruby", 52), file("utils/unreadable.rb", "Ruby", 0),
	}}}
	view := &pageView{}
	(&pageBuilder{data: data, links: newPageLinks(data)}).negatives(view)
	var said []string
	for _, language := range view.Unanalysed {
		line := fmt.Sprintf("%s %d %s:", language.Language, language.Count, language.Lines)
		for _, file := range language.Files {
			line += " " + file.Anchor.Path + " " + file.Lines
		}
		said = append(said, strings.TrimSpace(line))
	}
	if want := []string{
		"Tcl 3 2 236: test-redis.tcl 2 083 redis.tcl 131 utils/build-static-symbols.tcl 22",
		"Ruby 3 132: utils/redis-copy.rb 80 utils/redis-sha1.rb 52 utils/unreadable.rb",
	}; !slices.Equal(said, want) {
		t.Fatalf("unanalysed code = %q, want %q", said, want)
	}
}
