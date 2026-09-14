package clojureproject

import (
	"os"
	"path/filepath"
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
	foundUnderscore := false
	for _, relation := range index.Relations {
		from := objects[relation.FromID]
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
			foundShadow = true
			if relation.Resolution != p.ResolutionUnresolved {
				t.Fatalf("invented shadow target: %+v", relation)
			}
		}
	}
	if !foundCallback || !foundJava || !foundReader || !foundUnderscore {
		t.Fatalf("callback=%v Java=%v reader=%v underscore=%v", foundCallback, foundJava, foundReader, foundUnderscore)
	}
	if !foundCall || !foundLiteral || !foundShadow {
		t.Fatalf("call=%v literal=%v shadow=%v", foundCall, foundLiteral, foundShadow)
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
