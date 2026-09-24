package contracttest

import (
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"testing"
)

func TestClojureFixtureInventoryAndNativeGraph(t *testing.T) {
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
	if index.Target.Language != "clojure" || len(index.Target.Seeds) != 1 {
		t.Fatalf("ordinary target: %+v", index.Target)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.AssertExecutionScope(t, index, graph, "src/example/core.clj", 19, programindex.ObjectModule)
	adaptertest.AssertSQLQueryFacts(t, index, "src/example/core.clj", map[string]string{"SELECT id FROM direct_rows": "direct_rows", "DROP TABLE IF EXISTS %s": ""}, "create %s dir")
	// (-> path (str/replace "/" "-") (str/replace "/" "-"))
	// (str/replace (str/replace path "/" "-") "/" "-")
	// clj-kondo gives each threaded or nested form its own position. Java
	// instance chains such as (.. s (replace "/" "-")) carry no call pattern,
	// so they have no fact to compare.
	adaptertest.AssertCallSiteBoundaries(t, repository, result.Input, "src/example/core.clj", []adaptertest.CallSite{
		{Line: 37, Column: 12, Key: "clojure.string/replace", Text: "clojure.string.replace", Path: "/"},
		{Line: 37, Column: 34, Key: "clojure.string/replace", Text: "clojure.string.replace", Path: "/"},
		{Line: 40, Column: 3, Key: "clojure.string/replace", Text: "clojure.string.replace", Path: "/"},
		{Line: 40, Column: 16, Key: "clojure.string/replace", Text: "clojure.string.replace", Path: "/"},
	})
}
