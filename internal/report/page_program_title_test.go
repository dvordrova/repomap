package report

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A script program (one source file, no name from its build) is titled by
// its file and built from its file and what its code imports, followed
// through each imported file's own imports and never into a test; its one
// part, which takes the program's name, reads the same title. freqtrade's
// three build_helpers scripts shared one directory and each read
// "build_helpers.<module> (executable)", "Built from 373 files". A program
// its build names is titled by that name where its directory ends
// otherwise (the client's ft_client/freqtrade_client is freqtrade-client),
// and keeps its directory where it ends in the name (cmd/litestream).
func TestAScriptProgramIsItsFileAndWhatItImports(t *testing.T) {
	object := func(id, path string) programindex.Object {
		return programindex.Object{ID: id, Kind: programindex.ObjectModule, Location: &programindex.Location{Path: path, Line: 1, Column: 1}}
	}
	imports := func(id, from, path string, to ...string) programindex.Relation {
		return programindex.Relation{ID: id, Kind: programindex.RelationImports, FromID: from, ToIDs: to, Location: &programindex.Location{Path: path, Line: 3, Column: 5}}
	}
	script := programindex.Index{
		Target: programindex.Target{ID: "t2", Kind: "executable", Name: "build_helpers.create_command_partials",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "build_helpers/create_command_partials.py"}}, TestSources: []string{"tests/test_arguments.py"}},
		Objects: []programindex.Object{
			object("n1", "build_helpers/create_command_partials.py"), object("n2", "freqtrade/commands/arguments.py"),
			object("n3", "freqtrade/commands/cli_options.py"), object("n4", "freqtrade/worker.py"), object("n5", "tests/test_arguments.py"),
			{ID: "n6", Kind: "external_symbol"},
		},
		Relations: []programindex.Relation{
			imports("e1", "n1", "build_helpers/create_command_partials.py", "n2", "n6"),
			imports("e2", "n2", "freqtrade/commands/arguments.py", "n3"),
			imports("e3", "n5", "tests/test_arguments.py", "n2"),
			imports("e4", "n5", "freqtrade/commands/cli_options.py", "n5"),
		},
	}
	product := script
	product.Target = programindex.Target{ID: "t1", Kind: "executable", Name: "freqtrade", Executables: []string{"freqtrade"},
		Sources: []programindex.TargetSource{{FileRef: "f2", Path: "freqtrade/main.py"}}}
	builder := pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{product, script}}}}
	if got, want := builder.builtFrom("t2"), []string{"build_helpers/create_command_partials.py", "freqtrade/commands/arguments.py", "freqtrade/commands/cli_options.py"}; !slices.Equal(got, want) {
		t.Fatalf("the script is built from %q, want %q", got, want)
	}
	if got := builder.builtFrom("t1"); len(got) != 5 {
		t.Fatalf("the named program is built from %q", got)
	}
	client := programindex.Target{ID: "t4", Kind: "executable", Name: "freqtrade-client", Executables: []string{"freqtrade-client"},
		Sources: []programindex.TargetSource{{FileRef: "f3", Path: "ft_client/freqtrade_client/ft_client.py"}, {FileRef: "f4", Path: "ft_client/pyproject.toml"}}}
	extract := programindex.Target{ID: "t3", Kind: "executable", Name: "build_helpers.extract_config_json_schema",
		Sources: []programindex.TargetSource{{FileRef: "f5", Path: "build_helpers/extract_config_json_schema.py"}}}
	sections := []*pageSection{
		{ID: "t1", Name: "freqtrade", Kind: "executable", Root: "freqtrade", title: programTitle(product.Target, "freqtrade")},
		{ID: "t2", Name: "build_helpers.create_command_partials", Kind: "executable", Root: "build_helpers", title: programTitle(script.Target, "build_helpers")},
		{ID: "t3", Name: "build_helpers.extract_config_json_schema", Kind: "executable", Root: "build_helpers", title: programTitle(extract, "build_helpers")},
		{ID: "t4", Name: "freqtrade-client", Kind: "executable", Root: "ft_client/freqtrade_client", title: programTitle(client, "ft_client/freqtrade_client")},
		{ID: "t5", Name: "github.com/benbjohnson/litestream/cmd/litestream", Kind: "executable", Root: "cmd/litestream",
			title: programTitle(programindex.Target{Kind: "executable", Executables: []string{"litestream"}}, "cmd/litestream")},
	}
	labelSections(sections)
	var labels []string
	for _, section := range sections {
		labels = append(labels, componentTitle(section, sections))
	}
	if want := []string{"freqtrade", "build_helpers/create_command_partials.py", "build_helpers/extract_config_json_schema.py", "freqtrade-client", "cmd/litestream"}; !slices.Equal(labels, want) {
		t.Fatalf("titles = %q, want %q", labels, want)
	}
	folded := foldIndexes([]groupindex.Index{{Target: script.Target, Groups: []groupindex.Group{{ID: "g1", Title: "build_helpers.create_command_partials"}}},
		{Target: product.Target, Groups: []groupindex.Group{{ID: "g1", Title: "freqtrade"}}}})
	if folded[0].Groups[0].Title != "build_helpers/create_command_partials.py" || folded[1].Groups[0].Title != "freqtrade" {
		t.Fatalf("part titles = %q, %q", folded[0].Groups[0].Title, folded[1].Groups[0].Title)
	}
}
