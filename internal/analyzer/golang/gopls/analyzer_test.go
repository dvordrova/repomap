package gopls

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
)

type fakeRunner struct {
	outputs map[string]string
}

func (f fakeRunner) Run(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	output, ok := f.outputs[key]
	if !ok {
		return nil, fmt.Errorf("unexpected command %q", key)
	}
	return []byte(output), nil
}

func TestAnalyzeMarksPossibleAndStaticRelations(t *testing.T) {
	runner := fakeRunner{outputs: map[string]string{
		"version": "golang.org/x/tools/gopls v0.21.0\n",
		"workspace_symbol -matcher fuzzy Put": strings.Join([]string{
			"/repo/key.go:10:6-9 Put Function",
			"/repo/server.go:20:18-21 kv.Put Method",
		}, "\n"),
		"call_hierarchy /repo/key.go:10:6": strings.Join([]string{
			"caller[0]: ranges 30:2-5 in /repo/main.go from/to function main in /repo/main.go:25:6-10",
			"identifier: function Put in /repo/key.go:10:6-9",
			"callee[0]: ranges 11:2-5 in /repo/key.go from/to function Put in /repo/key.go:10:6-9",
			"callee[0]: ranges 12:2-7 in /repo/key.go from/to function Apply in /repo/apply.go:8:6-11",
		}, "\n"),
		"call_hierarchy /repo/server.go:20:18": "identifier: method kv.Put in /repo/server.go:20:18-21\n",
	}}
	analyzer := newWithRunner(Options{
		MaxSymbols:     10,
		MaxCallRoots:   2,
		CommandTimeout: time.Second,
	}, runner)

	graph, err := analyzer.Analyze(context.Background(), analysis.Request{RepoPath: "/repo", Query: "Put"})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	summary := graph.Summary()
	if summary.ByCertainty[evidence.CertaintyPossible] != 2 {
		t.Fatalf("possible relations = %d, want 2", summary.ByCertainty[evidence.CertaintyPossible])
	}
	if summary.ByCertainty[evidence.CertaintyStatic] != 3 {
		t.Fatalf("static relations = %d, want 3", summary.ByCertainty[evidence.CertaintyStatic])
	}
	assertRelation(t, graph, evidence.RelationResolvesTo, "Put")
}

func assertRelation(t *testing.T, graph evidence.Graph, kind evidence.RelationKind, targetName string) {
	t.Helper()

	entities := make(map[string]evidence.Entity, len(graph.Entities))
	for _, entity := range graph.Entities {
		entities[entity.ID] = entity
	}
	for _, relation := range graph.Relations {
		if relation.Kind == kind && entities[relation.To].Name == targetName {
			return
		}
	}
	t.Fatalf("relation %q to %q not found", kind, targetName)
}

func TestResolveExactSymbol(t *testing.T) {
	t.Parallel()

	symbols := []symbol{
		{Name: "KVServer.Put", Location: evidence.Location{Path: "generated.go", Line: 1}},
		{Name: "kvServer.Put", Location: evidence.Location{Path: "key.go", Line: 90}},
	}

	resolved, ok := resolveExactSymbol("kvServer.Put", symbols)
	if !ok {
		t.Fatal("resolveExactSymbol() did not resolve unique exact symbol")
	}
	if resolved.Location.Path != "key.go" {
		t.Fatalf("resolved path = %q, want key.go", resolved.Location.Path)
	}
}

func TestResolveExactSymbolRejectsAmbiguousName(t *testing.T) {
	t.Parallel()

	symbols := []symbol{
		{Name: "Run", Location: evidence.Location{Path: "a.go", Line: 1}},
		{Name: "Run", Location: evidence.Location{Path: "b.go", Line: 2}},
	}

	if _, ok := resolveExactSymbol("Run", symbols); ok {
		t.Fatal("resolveExactSymbol() resolved ambiguous symbol")
	}
}

func TestCanonicalizeHierarchyRootUsesWorkspaceSymbolIdentityAtSamePosition(t *testing.T) {
	t.Parallel()

	requested := symbol{
		Name:     "kvServer.Put",
		Kind:     evidence.EntityMethod,
		Location: evidence.Location{Path: "server/key.go", Line: 90, Column: 20},
	}
	reported := symbol{
		Name:     "Put",
		Kind:     evidence.EntityMethod,
		Location: evidence.Location{Path: "server/key.go", Line: 90, Column: 20},
	}

	canonical := canonicalizeHierarchyRoot(requested, reported)
	if canonical.Name != "kvServer.Put" {
		t.Fatalf("canonical name = %q, want kvServer.Put", canonical.Name)
	}
}

func TestParseWorkspaceSymbols(t *testing.T) {
	output := []byte(strings.Join([]string{
		"/repo/key.go:10:6-9 Put Function",
		"/repo/key.go:20:18-21 kv.Put Method",
		"Log: ignored diagnostic line",
	}, "\n"))

	symbols := parseWorkspaceSymbols(output)
	if len(symbols) != 2 {
		t.Fatalf("symbols = %d, want 2", len(symbols))
	}
	if symbols[1].Name != "kv.Put" || symbols[1].Kind != evidence.EntityMethod {
		t.Fatalf("symbols[1] = %#v", symbols[1])
	}
}

func TestParseImplementationLocations(t *testing.T) {
	locations := parseLocations([]byte(strings.Join([]string{
		"/repo/impl.go:14:6-12",
		"/repo/other.go:22:2-5",
		"Info: loading packages",
	}, "\n")))
	if len(locations) != 2 {
		t.Fatalf("locations = %d, want 2", len(locations))
	}
	if locations[0].Line != 14 || locations[0].Column != 6 {
		t.Fatalf("locations[0] = %#v", locations[0])
	}
}
