package contracttest

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

func TestClojureFixtureInventoryAndNativeGraph(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "clojure")
	targets, err := clojureproject.Scout(repository, "clojure")
	if err != nil || len(targets) != 1 {
		t.Fatalf("Clojure discovery: %v %v", targets, err)
	}
	result, err := sharedClojureFixture(t, root, repository, targets[0])
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
	assertNativeAdjacentCommentOwners(t, root, repository, index, "src/example/service.cljc", "example.service/documented-neighbor", "example.service/undocumented-neighbor")
	assertClojureJavaStaticCalls(t, index)
	assertClojureAnonymousArgumentCall(t, index)
	assertClojureReferAllKeepsTheNamespacesOwnVar(t, index)
	adaptertest.AssertExecutionScope(t, index, graph, "src/example/core.clj", 19, programindex.ObjectModule)
	// A call in a def's value is the var's, as a Go package-level variable
	// owns its initializer's calls (GO).
	adaptertest.AssertExecutionScope(t, index, graph, "src/example/core.clj", 140, programindex.ObjectVariable)
	adaptertest.AssertSQLQueryFacts(t, index, "src/example/core.clj", map[string]string{"SELECT id FROM direct_rows": "direct_rows", "DROP TABLE IF EXISTS %s": "", "SELECT 0 AS a": ""}, "create %s dir")
	assertOneStatementPerCall(t, index)
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

// (defmacro fresh-list [] `(ArrayList.)) with ArrayList imported: clj-kondo
// reports the quoted constructor as a call that names no method, which once
// became an outside symbol with no name and failed the whole index (metabase).
// It calls nothing; (java.util.UUID/randomUUID) in new-id is a static call.
func assertClojureJavaStaticCalls(t *testing.T, index programindex.Index) {
	t.Helper()
	byID := map[string]programindex.Object{}
	for _, object := range index.Objects {
		byID[object.ID] = object
	}
	calls := map[string][]string{}
	for _, relation := range index.Relations {
		from := byID[relation.FromID]
		if from.Name != "example.core/fresh-list" && from.Name != "example.core/new-id" {
			continue
		}
		for _, id := range relation.ToIDs {
			if to := byID[id]; to.External != nil {
				calls[from.Name] = append(calls[from.Name], string(relation.Kind)+" "+to.External.PackagePath+"/"+to.External.Name)
			}
		}
	}
	if len(calls["example.core/fresh-list"]) != 0 {
		t.Fatalf("a quoted constructor became a call: %v", calls["example.core/fresh-list"])
	}
	if !slices.Equal(calls["example.core/new-id"], []string{"invokes_external clojure.core/str", "calls java.util.UUID/randomUUID"}) {
		t.Fatalf("new-id's static call: %v", calls["example.core/new-id"])
	}
}

// Python's star facade in Clojure: example.facade refers every var of
// example.rates (`:refer :all`) and defines its own to-text, which core calls
// through an alias. The alias reaches the facade's own var, and the bare
// index-of the facade calls is the referred var of example.rates.
func assertClojureReferAllKeepsTheNamespacesOwnVar(t *testing.T, index programindex.Index) {
	t.Helper()
	byID := map[string]programindex.Object{}
	for _, object := range index.Objects {
		byID[object.ID] = object
	}
	var calls []string
	for _, relation := range index.Relations {
		from := byID[relation.FromID].Name
		if relation.Kind != programindex.RelationCalls || from != "example.core/facade-text" && from != "example.facade/to-text" {
			continue
		}
		call := from + " " + string(relation.Resolution)
		for _, id := range relation.ToIDs {
			if to := byID[id]; to.Location != nil {
				call += " " + to.Location.Path + ":" + to.Name
			}
		}
		calls = append(calls, call)
	}
	slices.Sort(calls)
	want := []string{
		"example.core/facade-text exact src/example/facade.clj:example.facade/to-text",
		"example.facade/to-text exact src/example/rates.clj:example.rates/index-of",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls through :refer :all = %q, want %q", calls, want)
	}
}

// (defn apply-each [fs] (map #(% 1) fs)): clj-kondo names no local for `%`,
// so the call's pattern had an empty selector and failed the whole index
// (metabase). The call through the function value keeps `%` as written.
func assertClojureAnonymousArgumentCall(t *testing.T, index programindex.Index) {
	t.Helper()
	byID := map[string]programindex.Object{}
	for _, object := range index.Objects {
		byID[object.ID] = object
	}
	var selectors []string
	for _, relation := range index.Relations {
		if byID[relation.FromID].Name != "example.core/apply-each" || relation.Dispatch != string(programindex.DispatchFunctionValue) {
			continue
		}
		for _, pattern := range relation.Patterns {
			selectors = append(selectors, pattern.Selector)
		}
	}
	if !slices.Equal(selectors, []string{"%"}) {
		t.Fatalf("the call of an anonymous function's argument: %v", selectors)
	}
}

// (defn zero-rows [] (str "SELECT 0 AS a" " UNION ALL" " SELECT 0 AS a")):
// the statement handed twice, apart only in spacing, became two identical
// facts at one call, and the atlas refused the graph (metabase).
func assertOneStatementPerCall(t *testing.T, index programindex.Index) {
	t.Helper()
	layer, err := facts.Build(facts.Input{Targets: []facts.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, fact := range layer.OfKind(facts.KindSQLQuery) {
		if fact.Value == "SELECT 0 AS a" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("zero-rows' statement is %d facts, want 1", count)
	}
}
