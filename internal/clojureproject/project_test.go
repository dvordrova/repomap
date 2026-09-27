package clojureproject

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	p "github.com/dvordrova/repomap/internal/programindex"
)

func TestNativeCumulativeProject(t *testing.T) {
	root, err := filepath.Abs("../../testdata/repositories/clojure")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	targets, err := Scout(repository, "clojure")
	if err != nil || len(targets) != 1 {
		t.Fatalf("targets: %v %v", targets, err)
	}
	result, err := Build(t.Context(), root, repository, targets[0])
	if err != nil {
		t.Fatal(err)
	}
	index, err := p.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]p.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	readLimit := false
	for _, relation := range index.Relations {
		if relation.Kind != p.RelationReads {
			continue
		}
		for _, target := range relation.ToIDs {
			if objects[target].Name != "example.service/source-limit" {
				continue
			}
			if objects[relation.FromID].Name != "example.core/read-limit" || objects[target].Kind != p.ObjectVariable || relation.Location == nil || relation.Location.Path != "src/example/core.clj" || relation.Location.Column < 1 {
				t.Fatalf("data read borrowed a shadowed name or lost its original var/site: %+v", relation)
			}
			readLimit = true
		}
	}
	if !readLimit {
		t.Fatal("native imported var read missing")
	}
	if len(index.Target.Seeds) != 1 {
		t.Fatalf("seeds: %+v", index.Target.Seeds)
	}
	if len(index.Target.TestSources) != 1 || index.Target.TestSources[0] != "test/example/service_test.clj" {
		t.Fatalf("tests: %v", index.Target.TestSources)
	}
	foundCall, foundLiteral, foundShadow := false, false, false
	foundCallback, foundJava, foundReader := false, false, false
	foundUnderscore, foundMacroArgument := false, false
	for _, relation := range index.Relations {
		from := objects[relation.FromID]
		if relation.Kind == p.RelationCalls && len(relation.ToIDs) == 1 && objects[relation.ToIDs[0]].Name == "example.core/read-limit" {
			// (ensure! (read-limit)) in ensured-limit: a call written in a
			// macro's argument keeps its own place and caller.
			if foundMacroArgument || from.Name != "example.core/ensured-limit" || relation.Resolution != p.ResolutionExact ||
				relation.Location == nil || relation.Location.Path != "src/example/core.clj" || relation.Location.Line != 50 || relation.Location.Column != 12 {
				t.Fatalf("call in a macro argument lost its place or caller: %+v", relation)
			}
			foundMacroArgument = true
		}
		if relation.Kind == p.RelationPassesCallback && from.Name == "example.core/greet-many" {
			if len(relation.ToIDs) != 1 || objects[relation.ToIDs[0]].Name != "example.service/greet" || relation.SourceArgumentID == "" {
				t.Fatalf("callback lost source argument: %+v", relation)
			}
			foundCallback = true
		}
		if relation.Kind != p.RelationCalls {
			continue
		}
		if from.Name == "example.service/underscore-handler" {
			foundUnderscore = true
			if relation.Resolution != p.ResolutionUnresolved || len(relation.ToIDs) != 0 || relation.Location == nil || relation.Location.Path != "src/example/service.cljc" {
				t.Fatalf("underscore parameter acquired a global callback: %+v", relation)
			}
		}
		if from.Name == "example.service/platform" {
			for _, to := range relation.ToIDs {
				if objects[to].External != nil && objects[to].External.PackagePath == "java.lang.System" {
					foundJava = true
				}
			}
		}
		if from.Name == "example.service/reader-arguments" {
			for _, pattern := range relation.Patterns {
				if len(pattern.Arguments) == 5 && pattern.Arguments[0].Value == "kept" {
					foundReader = true
				}
			}
		}
		if from.Name == "example.core/-main" {
			for _, to := range relation.ToIDs {
				if objects[to].Name == "example.service/deliver!" {
					foundCall = true
					for _, pattern := range relation.Patterns {
						for _, arg := range pattern.Arguments {
							if arg.Value == "greeting.txt" {
								foundLiteral = true
							}
						}
					}
				}
			}
		}
		if from.Name == "example.core/with-shadow" {
			// A call through a local is a call through a function value; it
			// stays unresolved even where the local shadows a var.
			foundShadow = true
			if relation.Resolution != p.ResolutionUnresolved || len(relation.ToIDs) != 0 || relation.Dispatch != p.DispatchFunctionValue {
				t.Fatalf("invented shadow target: %+v", relation)
			}
		}
	}
	if !foundCallback || !foundJava || !foundReader || !foundUnderscore {
		t.Fatalf("callback=%v Java=%v reader=%v underscore=%v", foundCallback, foundJava, foundReader, foundUnderscore)
	}
	if !foundCall || !foundLiteral || !foundShadow || !foundMacroArgument {
		t.Fatalf("call=%v literal=%v shadow=%v macro argument=%v", foundCall, foundLiteral, foundShadow, foundMacroArgument)
	}
	// Mirrors the pandas store-target idiom: an fn inside a set! target or a
	// binding default is its function's code, like the fn in an ordinary read.
	// So is Python's lambda in a function header: an fn in a parameter :or
	// default, a :pre condition or an fn's own default runs when it is called.
	wantStoreTarget := []string{
		"calls example.service/apply-handler exact +1 [(fn [value] (service/greet value)) row]",
		"calls example.service/greet exact +1 [value]",
	}
	for _, name := range []string{
		"example.core/handled", "example.core/mark-handled!", "example.core/handled-or-default",
		"example.core/handled-param", "example.core/checked-handled", "example.core/handled-by-default",
	} {
		var got []string
		for _, relation := range index.Relations {
			from := objects[relation.FromID]
			if from.Name != name || len(relation.ToIDs) == 0 || objects[relation.ToIDs[0]].External != nil {
				continue
			}
			var args []string
			for _, pattern := range relation.Patterns {
				for _, arg := range pattern.Arguments {
					args = append(args, arg.Origin.Text)
				}
			}
			got = append(got, fmt.Sprintf("%s %s %s %+d %v", relation.Kind, objects[relation.ToIDs[0]].Name, relation.Resolution, relation.Location.Line-from.Location.Line, args))
		}
		slices.Sort(got)
		if !slices.Equal(got, wantStoreTarget) {
			t.Fatalf("%s:\n have %q\n want %q", name, got, wantStoreTarget)
		}
	}
	// A var's metadata and a defn's attr-maps, before or after its arities,
	// run once when the namespace loads, as a Python decorator's arguments run
	// where the function is defined: their calls belong to the namespace,
	// while the calls in the function's body stay the function's.
	first := 0
	for _, object := range index.Objects {
		if object.Name == "example.core/routed-by-meta" {
			first = object.Location.Line
		}
	}
	var loaded []string
	for _, relation := range index.Relations {
		if relation.Kind != p.RelationCalls || len(relation.ToIDs) != 1 || objects[relation.ToIDs[0]].Name != "example.service/greet" ||
			relation.Location.Path != "src/example/core.clj" || relation.Location.Line < first {
			continue
		}
		loaded = append(loaded, objects[relation.FromID].Name+" "+relation.Patterns[0].Arguments[0].Origin.Text)
	}
	slices.Sort(loaded)
	wantLoaded := []string{
		"example.core attr", "example.core def", "example.core meta", "example.core multi", "example.core once", "example.core tail",
		"example.core/routed-by-attr-map row", "example.core/routed-by-attr-map row", "example.core/routed-by-meta row",
	}
	if first == 0 || !slices.Equal(loaded, wantLoaded) {
		t.Fatalf("load-time calls:\n have %q\n want %q", loaded, wantLoaded)
	}
	if err := result.Dependencies.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, object := range index.Objects {
		if object.External != nil && object.External.PackagePath == "js" {
			t.Fatal("ClojureScript branch leaked into JVM target")
		}
	}
	// Native extraction must produce a stable complete input on a warm run.
	again, err := Build(t.Context(), root, repository, targets[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.New(again.Input)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := p.Encode(index)
	b, _ := p.Encode(second)
	if string(a) != string(b) {
		t.Fatal("native graph changed on identical warm input")
	}
}

func TestRealRepositoryProbe(t *testing.T) {
	root := os.Getenv("REPOMAP_CLOJURE_PROBE")
	if root == "" {
		t.Skip("optional real repository probe")
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	targets, err := Scout(repository, filepath.Base(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		t.Logf("reading %s (%d files)", target.Selector, len(target.Files))
		result, err := Build(t.Context(), root, repository, target)
		if err != nil {
			t.Fatal(err)
		}
		index, err := p.New(result.Input)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d objects, %d relations, %d seeds", target.Selector, len(index.Objects), len(index.Relations), len(index.Target.Seeds))
	}
}
