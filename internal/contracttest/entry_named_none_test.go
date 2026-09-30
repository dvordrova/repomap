package contracttest

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/llm"
)

// When the model answers that no written word names an entry, no word the
// code picks names it over that answer (owner's decision, 2026-09-30:
// freqtrade's Query(…, description=…) had been named by its description).
// cli.py's weekly_levels route, asked its name and answered none, is named
// by its handler; its token query parameter, whose handler is not
// established and whose name was answered none, is no entry at all, never
// token, the first word its call wrote. Go, JS/TS, C and Clojure name their
// entries by the same reading step (atlas_boundaries.name), so the Python
// fixture checks it for all.
func TestAnEntryNoWrittenWordNamesIsNamedByItsHandlerOrIsNone(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "python")
	index := pythonLibraryIndex(t)
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		if symbol, _ := item["symbol"].(string); column == "binds" && strings.HasSuffix(symbol, "fastapi.FastAPI.get") {
			return "request", true
		}
		return "", false
	}}
	// The token parameter, given an alias, is a request's entry; weeks,
	// given only a description, is none (as freqtrade's reading answered).
	preset.read = func(column string, item map[string]any, _ []llm.Option) (string, bool) {
		if symbol, _ := item["symbol"].(string); column == "enters" && strings.HasSuffix(symbol, ".Query") {
			if literals, _ := item["literals"].([]any); slices.Contains(literals, any("token")) {
				return "request", true
			}
			return "none", true
		}
		return "", false
	}
	preset.named = func(words []map[string]any) any {
		for _, word := range words {
			if value, _ := word["value"].(string); value == "/api/weekly" || value == "Session token of the caller" {
				return "none"
			}
		}
		return nil
	}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	weekly := fixtureLine(t, "python", "src/fixture_app/cli.py", `@app.get("/api/weekly")`)
	var route string
	var parameters []string
	for _, row := range inputRows(projected, "src/fixture_app/cli.py") {
		if row.handler == "weekly_levels" && row.at == "src/fixture_app/cli.py:"+strconv.Itoa(weekly) {
			route = row.name
		}
		if row.handler == "" && (row.at == "src/fixture_app/cli.py:"+strconv.Itoa(weekly+2) || row.at == "src/fixture_app/cli.py:"+strconv.Itoa(weekly+3)) {
			parameters = append(parameters, row.name)
		}
	}
	asked := false
	for _, item := range preset.asked["atlas_boundaries.name"] {
		words, _ := item["words"].([]any)
		for _, word := range words {
			if value, _ := word.(map[string]any)["value"].(string); value == "Session token of the caller" {
				asked = true
			}
		}
	}
	if !asked {
		t.Fatal("token's name was never asked")
	}
	if route != "weekly_levels" {
		t.Fatalf("the route answered none is named %q, want its handler weekly_levels", route)
	}
	if len(parameters) > 0 {
		t.Fatalf("weekly_levels' query parameters %q: token, whose name was answered none, is named by a word the code picked", parameters)
	}
}
