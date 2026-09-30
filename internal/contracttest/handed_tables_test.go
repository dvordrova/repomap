package contracttest

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
)

// A table of words an established entry's handler looks up with part of
// what it was handed holds the entry's values: Python's level_title, the
// handler of GET /levels/{level}, calls
// LEVEL_TITLES.get(request.path_params["level"]), so easy and hard are
// listed under the route, each with its row as written, and the table is
// asked nothing. Clojure's on-key hands (:key event) on to command-for
// (TestCumulativeClojureKeywordHandoffsAndFutures). Go and JS/TS look a
// map or an object up by an index expression, which is no call, and
// record no table of words (GO, JSTS); C's command tables are registrations
// of their own rows (D1), so a lookup of one adds nothing (C).
func TestAHandlersTableLookupListsTheValuesItTakes(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "python")
	index := pythonLibraryIndex(t)
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		if symbol, _ := item["symbol"].(string); column == "binds" && strings.HasSuffix(symbol, "FastAPI.get") {
			return "request", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	// Each value is its key with what the key names: easy names "Easy level".
	names := map[string]string{}
	for _, operation := range projected.Operations {
		names[operation.ID] = strings.Join(append([]string{operation.Name}, operation.Names...), " -> ")
	}
	found := false
	for position, operation := range projected.Operations {
		if operation.Location.Path != "src/fixture_app/levels.py" || operation.HandlerUnknown {
			continue
		}
		found = true
		var values []string
		for _, id := range projected.Reach[position].SubArguments {
			values = append(values, names[id])
		}
		if !slices.Equal(values, []string{"easy -> Easy level", "hard -> Hard level"}) {
			t.Fatalf("%s's values: %q, want easy and hard with the titles they name", operation.Name, values)
		}
	}
	if !found {
		t.Fatalf("no route in levels.py: %+v", inputRows(projected, "src/fixture_app/levels.py"))
	}
	for _, item := range preset.asked["becomes"] {
		if item["table"] == "LEVEL_TITLES" {
			t.Fatal("LEVEL_TITLES, whose rows are a route's values, was asked")
		}
	}
	if written := writtenRows(projected, "src/fixture_app/levels.py"); written["easy"] != `"easy": "Easy level"` {
		t.Fatalf("easy's row as written: %q", written)
	}
}
