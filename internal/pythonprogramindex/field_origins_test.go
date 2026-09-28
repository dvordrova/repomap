package pythonprogramindex

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// A class's field stored once from an outside call holds that call's result
// wherever the class reads it: tool_cli.py's ServiceCommands stores
// argparse's parser in __init__ and the subcommand collection in build, so
// the calls on them are argparse's, each on the field it is made on, and the
// set_defaults of serve's parser hands run_serve over, as freqtrade's
// Arguments builds its subcommands. RebuiltParser stores its parser twice:
// the call on it stays unresolved.
func TestCumulativePythonFieldStoredOnceFromACallKeepsItsOrigin(t *testing.T) {
	const path = "src/fixture_app/tool_cli.py"
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
	name := func(id string) string { return objects[id].Name }
	lines := strings.Split(files[path], "\n")
	type call struct{ target, receiver, origin, result string }
	got := map[string]call{}
	results := map[string][]string{}
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != path || len(relation.Patterns) != 1 {
			continue
		}
		owner := objects[objects[relation.FromID].OwnerID].Name
		if owner != "ServiceCommands" && owner != "RebuiltParser" {
			continue
		}
		pattern := relation.Patterns[0]
		line := strings.TrimSpace(lines[relation.Location.Line-1])
		row := call{receiver: pattern.ReceiverID}
		if len(relation.ToIDs) == 1 {
			row.target = name(relation.ToIDs[0])
		}
		if len(pattern.ReceiverOriginIDs) == 1 {
			row.origin = name(pattern.ReceiverOriginIDs[0])
		}
		if pattern.ResultID != "" {
			results[pattern.ResultID] = append(results[pattern.ResultID], line)
		}
		got[line] = row
	}
	for line, row := range got {
		made := results[row.receiver]
		slices.Sort(made)
		row.result, row.receiver = strings.Join(made, " | "), name(row.receiver)
		got[line] = row
	}
	want := map[string]call{
		`self.parser = argparse.ArgumentParser("service")`: {target: "argparse.ArgumentParser"},
		`self._subparsers = self.parser.add_subparsers(dest="cmd")`: {target: "argparse.ArgumentParser.add_subparsers",
			receiver: "parser", origin: "argparse.ArgumentParser", result: `self.parser = argparse.ArgumentParser("service")`},
		`serve = self._subparsers.add_parser("serve")`: {target: "argparse.ArgumentParser.add_subparsers.add_parser",
			receiver: "_subparsers", origin: "argparse.ArgumentParser.add_subparsers", result: `self._subparsers = self.parser.add_subparsers(dest="cmd")`},
		`serve.set_defaults(func=run_serve)`: {target: "argparse.ArgumentParser.add_subparsers.add_parser.set_defaults",
			receiver: "serve", origin: "argparse.ArgumentParser.add_subparsers.add_parser", result: `serve = self._subparsers.add_parser("serve")`},
		`self.parser = argparse.ArgumentParser("first")`:  {target: "argparse.ArgumentParser"},
		`self.parser = argparse.ArgumentParser("second")`: {target: "argparse.ArgumentParser"},
		// Stored twice: either parser, so no outside member is claimed.
		`self.parser.add_argument("--again")`: {receiver: "parser",
			result: `self.parser = argparse.ArgumentParser("first") | self.parser = argparse.ArgumentParser("second")`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls on fields:\n have %+v\n want %+v", got, want)
	}
	var handed []string
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && objects[relation.FromID].Name == "build" {
			for _, id := range relation.ToIDs {
				handed = append(handed, name(id))
			}
		}
	}
	if !slices.Equal(handed, []string{"run_serve"}) {
		t.Fatalf("build hands over %v, want run_serve", handed)
	}
}
