package contracttest

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
)

// One handler registered by two calls of one kind under the same word is
// one input: the registration written first stands, and the other is kept
// among its aliases with its site and call as written. Under different
// words it is two inputs: a word a person types is its own (redis's
// smembers beside sinter, freqtrade's list-pairs beside list-markets).
// Python: cli.py's latest_levels at two routes (two inputs) and
// current_levels registered twice at one route (one). Go, C, JS/TS and
// Clojure fixtures register no handler twice; the fold is GroupsIndex's,
// the same for every language
// (TestAHandlerRegisteredTwiceUnderOneWordIsOneInput).
func TestAHandlerRegisteredTwiceIsOneInputPerWord(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "python")
	index := pythonLibraryIndex(t)
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		if symbol, _ := item["symbol"].(string); column == "binds" && (strings.HasSuffix(symbol, "fastapi.FastAPI.get") || strings.HasSuffix(symbol, "fastapi.FastAPI.api_route")) {
			return "request", true
		}
		return "", false
	}}
	// A route is named by its path, as a reader of its words names it.
	preset.named = func(words []map[string]any) any {
		for _, word := range words {
			if value, _ := word["value"].(string); strings.HasPrefix(value, "/") {
				return word["ref"]
			}
		}
		return nil
	}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	names := map[string]string{}
	for _, subject := range projected.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	got := map[string][]string{}
	for _, operation := range projected.Operations {
		handler := names[operation.SubjectID]
		if handler != "latest_levels" && handler != "current_levels" {
			continue
		}
		row := fmt.Sprintf("%s @%d", operation.Name, operation.Location.Line)
		for _, alias := range operation.Aliases {
			row += fmt.Sprintf(" | also @%d", alias.Location.Line)
		}
		got[handler] = append(got[handler], row)
	}
	latest := fixtureLine(t, "python", "src/fixture_app/cli.py", `@app.get("/api/levels/latest")`)
	current := fixtureLine(t, "python", "src/fixture_app/cli.py", `@app.get("/api/levels/current")`)
	slices.Sort(got["latest_levels"])
	want := map[string][]string{
		"latest_levels":  {fmt.Sprintf("/api/levels/latest @%d", latest), fmt.Sprintf("/api/levels/newest @%d", latest+1)},
		"current_levels": {fmt.Sprintf("/api/levels/current @%d | also @%d", current, current+1)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inputs %q, want %q", got, want)
	}
}
