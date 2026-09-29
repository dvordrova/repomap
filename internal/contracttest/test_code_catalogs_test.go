package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/extractors"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// Test code is testing, not the program. Each language fixture's test file
// writes `CREATE TABLE test_only_rows ...` handed to a callable: the facts
// see it (a data object from the database extractor, a sql_query where the
// adapter records the call), and none of it reaches the target's outbound
// calls or data, while the same kind of statement in ordinary code does. A
// Go test no load selects (a build tag) is held by no program, and its SQL
// is no program's data either, although the target's root holds its path.
//
// Go test sources are parsed declarations without calls, so a Go test makes
// data but no call fact. The database extractor reads no Clojure, so a
// Clojure test makes a call fact but no data. The JS/TS fixture has no SQL
// in ordinary code to compare with. The C adapter records no testing
// sources, so C has no equivalent.
func TestCumulativeTestCodeReachesNoOutboundOrData(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "0")
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOWORK", "off")
		root, repository := materializeFixtureRepository(t, "go")
		library := analyzeGoFixture(t, root, repository, goFixtureRootPackage, "cumulative-go-test-catalogs")
		index, err := goadapter.Build(repository, library.target, library.origins, library.direct, library.external, library.core, library.dynamic, library.tests)
		if err != nil {
			t.Fatal(err)
		}
		// root_optional_test.go builds only with repomap_optional_tests: no
		// load holds it, and its SQL is no program's either.
		assertTestCodeOffCatalogs(t, root, repository, index, "internal/storefixture/data_sources.go", "root_test.go", "root_optional_test.go")
	})
	t.Run("python", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "python")
		catalog, err := pythontarget.Discover(t.Context(), repository)
		if err != nil {
			t.Fatal(err)
		}
		var target pythontarget.Target
		for _, candidate := range catalog.Entries {
			if candidate.Kind == pythontarget.KindLibrary && candidate.ProjectDir == "." {
				target = candidate
			}
		}
		input, err := pythonprogramindex.BuildInput(t.Context(), repository, target)
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(input)
		if err != nil {
			t.Fatal(err)
		}
		assertTestCodeOffCatalogs(t, root, repository, index, "src/fixture_app/data_sources.py", "tests/test_facade.py")
	})
	t.Run("jsts", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "jsts")
		_, index, _, err := jstsproject.Build(t.Context(), repository, root)
		if err != nil {
			t.Fatal(err)
		}
		assertTestCodeOffCatalogs(t, root, repository, index, "", "src/market.test.ts")
	})
	t.Run("clojure", func(t *testing.T) {
		root, repository := materializeFixtureRepository(t, "clojure")
		targets, err := clojureproject.Scout(repository, "clojure")
		if err != nil || len(targets) != 1 {
			t.Fatalf("Clojure discovery: %v %v", targets, err)
		}
		result, err := clojureproject.Build(t.Context(), root, repository, targets[0])
		if err != nil {
			t.Fatal(err)
		}
		index, err := programindex.New(result.Input)
		if err != nil {
			t.Fatal(err)
		}
		assertTestCodeOffCatalogs(t, root, repository, index, "src/example/core.clj", "test/example/service_test.clj")
	})
}

// assertTestCodeOffCatalogs reads one fixture target with the database
// extractor's data, as an ordinary run does, and checks that the statements
// testSource (one of the target's testing sources) and each unheld file (a
// test no load selects) write are seen by the facts and reach neither the
// outbound calls nor the data; productPath's statements, when given, still
// reach them.
func assertTestCodeOffCatalogs(t *testing.T, root string, repository *corpus.Corpus, index programindex.Index, productPath, testSource string, unheld ...string) {
	t.Helper()
	if !slices.Contains(index.Target.TestSources, testSource) {
		t.Fatalf("%s is not a testing source of the target: %v", testSource, index.Target.TestSources)
	}
	tests := append([]string{testSource}, unheld...)
	var files []string
	for _, entry := range repository.Entries() {
		files = append(files, entry.Path)
	}
	response, err := extractors.Database(t.Context(), extractors.Request{Version: extractors.Version, Root: root, Files: files})
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{
		Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}},
		Extractions: []facts.Extraction{{Name: "database", Nodes: response.Nodes, Links: response.Links}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, testPath := range tests {
		seen := false
		for _, fact := range layer.Facts {
			seen = seen || fact.Kind == facts.KindEntity && fact.Data != nil && fact.Path == testPath ||
				fact.Kind == facts.KindSQLQuery && fact.Anchor != nil && fact.Anchor.Path == testPath
		}
		if !seen {
			t.Fatalf("the facts do not see the SQL %s writes", testPath)
		}
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	preset := &inputsPreset{decide: func(string, map[string]any, []string) (string, bool) { return "", false }}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: index.Target.Language, Kind: index.Target.Kind, Name: index.Target.Name, Root: "."}, root, preset)
	product := false
	for _, call := range projected.Outbound {
		if slices.Contains(index.Target.TestSources, call.Location.Path) || slices.Contains(tests, call.Location.Path) {
			t.Fatalf("a test's call is an outbound call of the program: %+v", call)
		}
		product = product || call.Location.Path == productPath
	}
	for _, record := range projected.Data {
		if slices.Contains(index.Target.TestSources, record.Path) || slices.Contains(tests, record.Path) {
			t.Fatalf("a test's SQL is data of the program: %s:%d %+v", record.Path, record.Line, record.Data)
		}
		product = product || record.Path == productPath
	}
	if productPath != "" && !product {
		t.Fatalf("the SQL %s writes reaches neither the outbound calls nor the data", productPath)
	}
}
