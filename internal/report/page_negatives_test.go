package report

import (
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
