package pythonprogramindex

import (
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
