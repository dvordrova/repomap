package pythonprogramindex

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// workers.py builds and runs its workers the way freqtrade's start_trading
// runs its Worker: `worker = None`, then `worker = Worker(name, 2)` and
// `worker.run()`. Calling a class is a construct call of the class that
// also runs its __init__, its own or the one it inherits; a name assigned
// once from it (a store of None aside) makes a call on it the class's
// method, an inherited one included. A name stored twice keeps its call
// unresolved, and a class with no __init__ in the repository runs none.
func TestCumulativePythonConstructorCallsRunInitAndTypeTheirName(t *testing.T) {
	const path = "src/fixture_app/workers.py"
	files := cumulativePythonSources(t, path)
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	name := func(id string) string {
		object := objects[id]
		if owner, ok := objects[object.OwnerID]; ok && object.Kind == programindex.ObjectMethod {
			return owner.Name + "." + object.Name
		}
		return object.Name
	}
	lines := strings.Split(files[path], "\n")
	got := map[string][]string{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Location.Path != path {
			continue
		}
		caller := objects[relation.FromID].Name
		if !strings.HasPrefix(caller, "start_") && caller != "make_plain" {
			continue
		}
		row := string(relation.Resolution)
		for _, id := range relation.ToIDs {
			row += " " + name(id)
		}
		if relation.Invocation != "" {
			row += " (" + relation.Invocation + ")"
		}
		line := strings.TrimSpace(lines[relation.Location.Line-1])
		got[line] = append(got[line], row)
	}
	for line := range got {
		// Two relations at one site keep no order of their own.
		if rows := got[line]; len(rows) == 2 && rows[0] > rows[1] {
			rows[0], rows[1] = rows[1], rows[0]
		}
	}
	want := map[string][]string{
		"worker = Worker(name, 2)":   {"exact Worker (construct)", "exact Worker.__init__ (construct)"},
		"worker.run()":               {"exact BaseWorker.run"},
		"worker.step()":              {"exact Worker.step"},
		"quiet = QuietWorker(name)":  {"exact BaseWorker.__init__ (construct)", "exact QuietWorker (construct)"},
		"return quiet.run()":         {"exact BaseWorker.run"},
		"chosen = Worker(name, 1)":   {"exact Worker (construct)", "exact Worker.__init__ (construct)"},
		"chosen = QuietWorker(name)": {"exact BaseWorker.__init__ (construct)", "exact QuietWorker (construct)"},
		// Stored twice: either worker, so no method is claimed.
		"return chosen.run()": {"unresolved"},
		"return Plain()":      {"exact Plain (construct)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls in workers.py:\n have %q\n want %q", got, want)
	}
}

// A field an __init__ stores once from its own parameter holds what every
// construction of the class hands it: Rpc's bot is the Bot that Bot's own
// __init__ hands Manager, which hands it on through its parameter
// (freqtrade's RPC._freqtrade, built by RPCManager(self) in FreqtradeBot);
// Replay's engine is a Bot or a Backtest; Loose's engine is a value
// nothing types, so its call stays unresolved.
func TestCumulativePythonFieldStoredFromAConstructorParameterIsWhatItsConstructionsHand(t *testing.T) {
	const path = "src/fixture_app/workers.py"
	files := cumulativePythonSources(t, path)
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	got := map[string]string{}
	for _, relation := range index.Relations {
		caller := objects[relation.FromID]
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Location.Path != path ||
			caller.Name != "force_entry" && caller.Name != "replay" && caller.Name != "replay_loose" {
			continue
		}
		row := string(relation.Resolution)
		for _, id := range relation.ToIDs {
			row += " " + objects[objects[id].OwnerID].Name + "." + objects[id].Name
		}
		witnessed := 0
		for _, witness := range relation.Witnesses {
			if witness.Kind == "interface_field_assignment" && witness.Location != nil && witness.Location.Path == path {
				witnessed++
			}
		}
		got[caller.Name] = fmt.Sprintf("%s (%d handed)", row, witnessed)
	}
	want := map[string]string{
		"force_entry":  "exact Bot.execute_entry (1 handed)",
		"replay":       "alternatives Bot.execute_entry Backtest.execute_entry (2 handed)",
		"replay_loose": "unresolved (0 handed)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls on fields stored from a constructor parameter:\n have %q\n want %q", got, want)
	}
}
