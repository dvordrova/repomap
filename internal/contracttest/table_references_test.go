package contracttest

import (
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/programindex"
)

// ARGS_SERVE and ARGS_INIT are keys build_args (handed them by keyword and
// by position, from build_subcommands and build_serve) and init_flags look
// up in OPTIONS, and NO_CONFIG and KNOWN
// are only tested for a subcommand's membership, "prune" naming no
// subcommand: their reads say so with ProgramIndex's shared witnesses, and
// none of them is asked what its rows become. HELP, looked up with its own
// rows, COMMANDS, looked up only under a condition and handed through a
// class, and READ_ONLY and WRITES, tested case by case, stay asked. A
// preset accepting every asked table as commands makes inputs of the asked
// tables' rows alone. C tables record neither form (no string subscript, no
// membership operator), and Go, JS/TS and Clojure record no tables of names
// (PYTHON, C).
func TestPythonTablesNamingAnotherTablesRowsAreNoInputs(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "python")
	index := pythonLibraryIndex(t)
	names := map[string]string{}
	for _, object := range index.Objects {
		names[object.ID] = object.Name
	}
	reads := map[string][]string{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationReads || relation.Location == nil || relation.Location.Path != "src/fixture_app/dispatch.py" {
			continue
		}
		for _, to := range relation.ToIDs {
			for _, witness := range relation.Witnesses {
				if witness.Kind != programindex.WitnessKeys && witness.Kind != programindex.WitnessMembership {
					continue
				}
				read := names[relation.FromID] + " " + witness.Kind
				if witness.ObjectID != "" {
					read += " " + names[witness.ObjectID]
				}
				reads[names[to]] = append(reads[names[to]], read)
			}
		}
	}
	for name := range reads {
		slices.Sort(reads[name])
	}
	want := map[string][]string{
		"ARGS_SERVE": {"build_serve keys OPTIONS", "build_subcommands keys OPTIONS"},
		"ARGS_INIT":  {"build_subcommands keys OPTIONS", "init_flags keys OPTIONS"},
		"NO_CONFIG":  {"needs_config membership"},
		"KNOWN":      {"run_known membership"},
		"HELP":       {"described_commands membership"},
	}
	if !reflect.DeepEqual(reads, want) {
		t.Fatalf("table reads: %q, want %q", reads, want)
	}

	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		if _, table := item["table"]; column == "becomes" && table {
			return "command", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	var asked []string
	for _, item := range preset.asked["becomes"] {
		if name, ok := item["table"].(string); ok && item["file"] == "src/fixture_app/dispatch.py" {
			asked = append(asked, name)
		}
	}
	slices.Sort(asked)
	wantAsked := []string{"COMMANDS", "HELP", "OPTIONS", "READ_ONLY", "REQUIRED", "WRITES"}
	if !reflect.DeepEqual(asked, wantAsked) {
		t.Fatalf("tables asked: %q, want %q", asked, wantAsked)
	}
	var declaring []string
	for _, row := range inputRows(projected, "src/fixture_app/dispatch.py") {
		if row.kind != "command" || row.name == "prune" {
			t.Fatalf("dispatch.py's input %+v", row)
		}
		if !slices.Contains(declaring, row.declaredBy) {
			declaring = append(declaring, row.declaredBy)
		}
	}
	slices.Sort(declaring)
	if !reflect.DeepEqual(declaring, wantAsked) {
		t.Fatalf("dispatch.py's inputs are declared by %q, want %q", declaring, wantAsked)
	}
}

// A row of an accepted table keeps its own row as written, never the
// whole table: freqtrade's 124 option rows each carried the 19.7 KB
// AVAILABLE_CLI_OPTIONS dict, 3.08 MB of its 11.8 MB page. C's rows keep
// their braces (TestCFixtureInputsKeepTheirRegistrationAsWritten).
func TestATableRowIsWrittenAsItsOwnRow(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "python")
	index := pythonLibraryIndex(t)
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		if column == "becomes" && item["table"] == "OPTIONS" {
			return "command", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "python", Kind: "library", Name: index.Target.Name, Root: "."}, root, preset)
	written := writtenRows(projected, "src/fixture_app/dispatch.py")
	want := map[string]string{
		"verbose": `"verbose": Opt("-v", "--verbose", help="print more")`,
		"force":   `"force": Opt("-f", "--force", help="overwrite files")`,
	}
	if !reflect.DeepEqual(written, want) {
		t.Fatalf("OPTIONS rows as written: %q, want %q", written, want)
	}
}
