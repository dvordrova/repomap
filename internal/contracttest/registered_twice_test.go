package contracttest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
)

// One handler registered by two calls of one kind is one input: the
// registration written first stands, and the other is kept among its
// aliases with its word and site (freqtrade's CallbackQueryHandler(
// self._profit, pattern="update_profit$") under CommandHandler("profit",
// self._profit); redis's smembers row under sinter's). Python: cli.py's
// latest_levels, decorated with two routes. Go, C, JS/TS and Clojure
// fixtures register no handler twice; the fold is GroupsIndex's, the same
// for every language (TestAHandlerRegisteredTwiceIsOneInputWithItsOtherRegistration).
func TestAHandlerRegisteredTwiceIsOneInput(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "python")
	index := pythonLibraryIndex(t)
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		if symbol, _ := item["symbol"].(string); column == "binds" && strings.HasSuffix(symbol, "fastapi.FastAPI.get") {
			return "request", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	names := map[string]string{}
	for _, subject := range projected.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	var got []string
	for _, operation := range projected.Operations {
		if names[operation.SubjectID] != "latest_levels" {
			continue
		}
		row := fmt.Sprintf("%s @%d", operation.Name, operation.Location.Line)
		for _, alias := range operation.Aliases {
			row += fmt.Sprintf(" | also %s @%d: %s", alias.Name, alias.Location.Line, alias.Written)
		}
		got = append(got, row)
	}
	line := fixtureLine(t, "python", "src/fixture_app/cli.py", `@app.get("/api/levels/latest")`)
	if len(got) != 1 || !strings.HasPrefix(got[0], fmt.Sprintf("get @%d | also get @%d: ", line, line+1)) || !strings.Contains(got[0], "/api/levels/newest") {
		t.Fatalf("latest_levels' inputs %q", got)
	}
}
