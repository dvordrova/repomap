package jstsproject

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCumulativeJSTSIterationKeepsCompilerElementAuthority(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "shared/contracts.ts", "src/market-worker.js", "src/facade-exports/exchange.ts", "src/facade-exports/exchange-index.ts", "src/facade-exports/resolver.ts", "src/facade-exports/index.ts", "src/facade-exports/consumer.ts"}
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
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	wanted := map[string]bool{"typedIteration": true, "typedJSIteration": true, "unknownIteration": false, "unknownJSIteration": false,
		"typedUnderscore": true, "typedJSUnderscore": true, "unknownUnderscore": false, "unknownJSUnderscore": false}
	seen := map[string]bool{}
	for _, relation := range index.Relations {
		name := objects[relation.FromID].Name
		known, ok := wanted[name]
		if !ok || relation.Kind != programindex.RelationCalls {
			continue
		}
		seen[name] = true
		if !known {
			if relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
				t.Fatalf("%s borrowed a type: %+v", name, relation)
			}
			continue
		}
		resolution := programindex.ResolutionExact
		if name == "typedJSIteration" || name == "typedJSUnderscore" {
			resolution = programindex.ResolutionAlternatives
		}
		if relation.Resolution != resolution || len(relation.ToIDs) != 1 || relation.Location == nil {
			t.Fatalf("%s lost compiler call authority: %+v", name, relation)
		}
		method := objects[relation.ToIDs[0]]
		if method.Location == nil || method.Location.Path != "src/facade-exports/exchange.ts" {
			t.Fatalf("%s borrowed a same-named method: %+v", name, method)
		}
	}
	if len(seen) != len(wanted) {
		t.Fatalf("iteration coverage: %v", seen)
	}
}
