package jstsproject

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCumulativeJSTSExplicitBarrelsRetainFactoryCallbackAuthority(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/facade-exports/exchange.ts", "src/facade-exports/exchange-index.ts", "src/facade-exports/resolver.ts", "src/facade-exports/index.ts", "src/facade-exports/consumer.ts"}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	var callback string
	for _, object := range index.Objects {
		if object.Location != nil && object.Location.Path == "src/facade-exports/exchange.ts" && strings.HasSuffix(object.Name, "resetStream") {
			callback = object.ID
		}
	}
	if callback == "" {
		t.Fatal("compiler lost original exchange method")
	}
	var known, unknown int
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != "src/facade-exports/consumer.ts" {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.Selector != "setInterval" {
				continue
			}
			for _, argument := range pattern.Arguments {
				if argument.Position == 2 && relation.Location.Line != 15 && (argument.Origin == nil || (argument.Origin.Text != "60000" && argument.Origin.Text != "120000")) {
					t.Fatalf("native timer argument origin changed: %+v", argument.Origin)
				}
				if argument.Position != 1 {
					continue
				}
				if relation.Location.Line == 15 {
					unknown++
					if len(argument.ObjectIDs) != 0 || argument.Resolution != programindex.ResolutionUnresolved {
						t.Fatalf("unknown receiver borrowed same-named method: %+v", argument)
					}
					continue
				}
				known++
				if len(argument.ObjectIDs) != 1 || argument.ObjectIDs[0] != callback || argument.Resolution != programindex.ResolutionExact {
					t.Fatalf("compiler resolved barrel/factory method changed identity: %+v", argument)
				}
			}
		}
	}
	if known != 2 || unknown != 1 {
		t.Fatalf("barrel callbacks known=%d unknown=%d", known, unknown)
	}
	graph, err := places.Build(places.Input{Revision: strings.Repeat("a", 40), Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != "src/facade-exports/exchange.ts" || !strings.HasSuffix(place.Symbol.Decl.Name, "resetStream") {
			continue
		}
		if len(place.Symbol.Bindings) != 2 {
			t.Fatalf("barrel method graph bindings=%+v", place.Symbol.Bindings)
		}
		for _, binding := range place.Symbol.Bindings {
			if binding.Resolution != "exact" || binding.Path != "src/facade-exports/consumer.ts" || binding.Detail != "argument 1 of setInterval" || (binding.Line != 9 && binding.Line != 10) {
				t.Fatalf("barrel callback lost compiler authority or original registration: %+v", binding)
			}
		}
		return
	}
	t.Fatal("original exchange method absent from consuming graph")
}

func TestCumulativeJSTSTypedParameterKeepsCompilerMethodAuthority(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/facade-exports/exchange.ts", "src/facade-exports/exchange-index.ts", "src/facade-exports/resolver.ts", "src/facade-exports/index.ts", "src/facade-exports/consumer.ts"}
	for _, path := range tracked {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(data))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	known, unknown := 0, 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Location.Path != "src/facade-exports/consumer.ts" || relation.Location.Line < 24 {
			continue
		}
		if relation.Location.Line == 26 {
			known++
			if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
				t.Fatalf("written native type lost compiler method: %+v", relation)
			}
			for _, object := range index.Objects {
				if object.ID == relation.ToIDs[0] && (object.Location == nil || object.Location.Path != "src/facade-exports/exchange.ts") {
					t.Fatalf("same-named class borrowed method: %+v", object)
				}
			}
		} else if relation.Location.Line == 29 {
			unknown++
			if relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
				t.Fatal("any parameter borrowed a method")
			}
		}
	}
	if known != 1 || unknown != 1 {
		t.Fatalf("typed calls known%d unknown%d", known, unknown)
	}
}

// Python's star facade (a module that star-imports another and then
// declares its own helper, called through a module alias) in TypeScript:
// `export * from` plus the module's own export, called through `import * as`.
// The compiler resolves both the module's own export and the one `export *`
// passes on to their declarations.
func TestCumulativeJSTSStarBarrelKeepsItsOwnExport(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/facade-exports/star-rates.ts", "src/facade-exports/star-index.ts", "src/facade-exports/star-consumer.ts"}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]string{}
	for _, object := range index.Objects {
		if object.Location != nil && object.Kind == programindex.ObjectFunction {
			declared[object.ID] = object.Location.Path + ":" + object.Name
		}
	}
	var calls []string
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Location.Path != "src/facade-exports/star-consumer.ts" {
			continue
		}
		call := string(relation.Resolution)
		for _, id := range relation.ToIDs {
			call += " " + declared[id]
		}
		calls = append(calls, call)
	}
	want := []string{"exact src/facade-exports/star-index.ts:toText", "exact src/facade-exports/star-rates.ts:getIndex"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls through the star barrel = %q, want %q", calls, want)
	}
}
