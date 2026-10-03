package contracttest

import (
	"fmt"
	"slices"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// ownerFixture is one language fixture's program, its corpus and its facts.
type ownerFixture struct {
	repository *corpus.Corpus
	index      programindex.Index
	layer      facts.Result
}

func ownerFacts(t *testing.T, repository *corpus.Corpus, index programindex.Index) ownerFixture {
	t.Helper()
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	return ownerFixture{repository: repository, index: index, layer: layer}
}

func ownerPython(t *testing.T) ownerFixture {
	t.Helper()
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	input, err := pythonprogramindex.BuildInput(t.Context(), repository, pythonFixtureTarget(t, catalog))
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	return ownerFacts(t, repository, index)
}

func ownerGo(t *testing.T) ownerFixture {
	t.Helper()
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, root)
	authorities := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "outside-owner")
	input, err := goadapter.BuildInput(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	return ownerFacts(t, repository, index)
}

func ownerJSTS(t *testing.T) ownerFixture {
	t.Helper()
	root, repository := materializeFixtureRepository(t, "jsts")
	_, index, _, err := jstsproject.Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	return ownerFacts(t, repository, index)
}

func ownerClojure(t *testing.T) ownerFixture {
	t.Helper()
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
	return ownerFacts(t, repository, index)
}

// factsAt lists one kind's facts in the files named, as "path:line key
// resolution".
func (fixture ownerFixture) factsAt(kind facts.Kind, files ...string) []string {
	var rows []string
	for _, fact := range fixture.layer.OfKind(kind) {
		if fact.Anchor != nil && slices.Contains(files, fact.Anchor.Path) {
			rows = append(rows, fmt.Sprintf("%s:%d %s %s", fact.Anchor.Path, fact.Anchor.Line, fact.Key, fact.Resolution))
		}
	}
	sort.Strings(rows)
	return rows
}

func expectRows(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %q, want %q", what, got, want)
	}
}

// declared lists the reader's declarations places makes of one file.
func (fixture ownerFixture) declared(t *testing.T, path string) []string {
	t.Helper()
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: fixture.index, Root: "."}}, Facts: fixture.layer})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, place := range graph.Places {
		if place.Symbol != nil && place.Path == path {
			names = append(names, place.Symbol.Decl.Name)
		}
	}
	sort.Strings(names)
	return names
}

// The 2026-10-03 review's A1–A3, one fixture build per language:
//
//   - A1: a setting read and a dynamic execution are calls of particular
//     outside functions, which the callee's owner as the native graph
//     resolves the call establishes, never the call's word. A function the
//     repository declares under the name getenv, Field, eval or exec is its
//     own code, and a call the graph leaves unresolved stays unknown. C
//     keeps its existing coverage: net.c's getenv("KVD_BACKLOG") is the
//     server's read (TestCFixtureProvesWhatEachProgramNeverRuns), and C
//     declares no evaluating function, so a repository function named eval
//     is its own code there too (facts' TestCFacts).
//   - A2: a route keeps the path its registration wrote unless a call
//     states a prefix for it, by position or under a keyword whose last
//     word is prefix; description="/docs" and docs_url="/docs" are values.
//     The other languages state a mount's prefix by position (JavaScript's
//     root.use("/api/v1", router) in route-mounts.ts, kept by jstsproject's
//     assertCumulativeJSTSRuntimeRegistrations); Go and C write no keyword
//     arguments, a JavaScript options object reaches no keyword, and the
//     Clojure fixture mounts no router: none has a keyword equivalent.
//   - A3: a reader's declarations come from the native kind, ownership and
//     anonymity, never a name's characters: a public price$ and _token are
//     declarations like price and token, whether or not their bodies call
//     anything, and a Go function literal is anonymous by the adapter's
//     mark, not by its Open$1 name. Python names hold no "$"; Go keeps no
//     package-level variable as a declaration and a Go identifier holds no
//     "$". In C a "$" is a compiler extension and a file-scope name starting
//     with an underscore is reserved to the implementation (C11 7.1.3), so
//     the C fixture keeps no such declaration.
func TestEveryLanguageDecidesByTheCalleeAStatedPrefixAndAnonymity(t *testing.T) {
	t.Run("python", func(t *testing.T) {
		fixture := ownerPython(t)
		const (
			lookalikes = "src/fixture_app/lookalike_names.py"
			readers    = "src/fixture_app/outside_readers.py"
			classScope = "src/fixture_app/class_scope.py"
			star       = "src/fixture_app/star_evaluation.py"
			runtime    = "src/fixture_app/runtime_registrations.py"
		)
		// The module's own getenv and Field read nothing; os.environ.get,
		// imported or not, and a BaseSettings field's env do; a BaseModel
		// field's env is metadata.
		expectRows(t, "config reads", fixture.factsAt(facts.KindConfigRead, lookalikes, readers),
			lookalikes+":32 FIXTURE_LOOKALIKE_SETTING exact",
			lookalikes+":36 FIXTURE_IMPORTED_ENVIRON_SETTING exact",
			readers+":13 FIXTURE_SERVICE_PORT exact")
		// The module's own eval and exec evaluate nothing; a method's bare
		// eval is the builtin, not its class's method; an eval a star
		// import may bind is unknown; yaml.load builds code with yaml's
		// Loader and reads data with SafeLoader.
		expectRows(t, "dynamic executions", fixture.factsAt(facts.KindDynamicExecution, lookalikes, readers, classScope, star, runtime),
			classScope+":16 eval exact",
			readers+":25 yaml.load exact",
			runtime+":95 eval exact")
		// A class body's range is no name its method sees: the call is the
		// builtin's, never a call of the class's own range.
		for _, relation := range fixture.index.Relations {
			if relation.Location == nil || relation.Location.Path != classScope || relation.Location.Line != 15 {
				continue
			}
			for _, id := range relation.ToIDs {
				if object := programIndexObjectByID(fixture.index, id); object.Name == "range" {
					t.Fatalf("range() in a method resolved to its class's method: %+v", relation)
				}
			}
		}
		paths := map[string]string{}
		for _, fact := range fixture.layer.OfKind(facts.KindRegistration) {
			if fact.Anchor != nil && fact.Anchor.Path == "src/fixture_app/route_metadata.py" && fact.Symbol != "" {
				paths[fact.Symbol] = fact.Path + " " + string(fact.Resolution)
			}
		}
		want := map[string]string{"described_ping": "/ping exact", "prefixed_ping": "/v2/ping exact", "documented_health": "/health exact"}
		if fmt.Sprint(paths) != fmt.Sprint(want) {
			t.Fatalf("route_metadata.py registers %v, want %v", paths, want)
		}
		if names := fixture.declared(t, lookalikes); !slices.Contains(names, "_token") {
			t.Fatalf("lookalike_names.py declares %v without _token", names)
		}
		if names := fixture.declared(t, "src/fixture_app/exports.py"); !slices.Contains(names, "__all__") {
			t.Fatalf("exports.py declares %v without __all__", names)
		}
	})
	t.Run("go", func(t *testing.T) {
		fixture := ownerGo(t)
		const lookalikes = "cmd/app/lookalike_names.go"
		// eval, Getenv and statusLedger.exec are the program's own; only
		// os.Getenv reads the environment, and Go evaluates no code.
		expectRows(t, "config reads", fixture.factsAt(facts.KindConfigRead, lookalikes),
			lookalikes+":22 FIXTURE_LOOKALIKE_SETTING exact")
		expectRows(t, "dynamic executions", fixture.factsAt(facts.KindDynamicExecution, lookalikes))
		anonymous := map[string]bool{}
		for _, object := range fixture.index.Objects {
			if object.Kind == programindex.ObjectFunction && object.Location != nil && object.Location.Path == "cmd/app/main.go" {
				anonymous[object.Name] = object.Anonymous
			}
		}
		// markExitRows' function literals are anonymous; markExitRows and
		// main are named.
		if !anonymous["markExitRows$1"] || anonymous["markExitRows"] || anonymous["main"] {
			t.Fatalf("cmd/app/main.go's anonymity = %v", anonymous)
		}
		if names := fixture.declared(t, "cmd/app/main.go"); !slices.Contains(names, "markExitRows") || !slices.Contains(names, "main") {
			t.Fatalf("cmd/app/main.go declares %v", names)
		}
	})
	t.Run("jsts", func(t *testing.T) {
		fixture := ownerJSTS(t)
		const (
			lookalikes = "src/lookalike-names.ts"
			runtime    = "src/runtime-registrations.ts"
		)
		// getenv and exec are the module's own and RegExp's exec matches
		// text; the platform's eval and Function build code. A program
		// reads process.env as a property, no call: no config read.
		expectRows(t, "config reads", fixture.factsAt(facts.KindConfigRead, lookalikes))
		expectRows(t, "dynamic executions", fixture.factsAt(facts.KindDynamicExecution, lookalikes, runtime),
			lookalikes+":19 new Function exact",
			runtime+":25 eval exact")
		expectRows(t, "declaration-names.ts declarations", fixture.declared(t, "src/declaration-names.ts"),
			"_token", "active$", "price", "price$", "token")
	})
	t.Run("clojure", func(t *testing.T) {
		fixture := ownerClojure(t)
		const lookalikes = "src/example/lookalike.clj"
		// The namespace's own eval and getenv are its code; the JVM's
		// System/getenv and clojure.core/eval are the real ones.
		expectRows(t, "config reads", fixture.factsAt(facts.KindConfigRead, lookalikes),
			lookalikes+":16 FIXTURE_LOOKALIKE_SETTING exact")
		expectRows(t, "dynamic executions", fixture.factsAt(facts.KindDynamicExecution, lookalikes),
			lookalikes+":19 eval exact")
		names := fixture.declared(t, lookalikes)
		for _, name := range []string{"example.lookalike/price$", "example.lookalike/_token"} {
			if !slices.Contains(names, name) {
				t.Fatalf("lookalike.clj declares %v without %s", names, name)
			}
		}
	})
}
