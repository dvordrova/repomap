package workspaceedgeselection

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	packageA    = "example.com/repo/a"
	packageB    = "example.com/repo/b"
	packageC    = "example.com/repo/c"
	packageSelf = "example.com/repo/self"
)

var benchmarkSelectionError error

func TestSelectionPreservesExactOrderDuplicatesAndAuthority(t *testing.T) {
	graph := newSelectionTestGraph(t, []gofacts.Edge{
		{From: packageA, To: packageB},
		{From: packageC, To: packageA},
		{From: packageSelf, To: packageSelf},
	})
	candidates := []Candidate{
		{From: packageC, To: packageA},
		{From: packageA, To: packageB},
		{From: packageC, To: packageA},
		{From: packageSelf, To: packageSelf},
	}
	selection, err := New(Input{Graph: graph, Candidates: candidates})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := []Edge{
		{From: packageC, To: packageA},
		{From: packageA, To: packageB},
		{From: packageC, To: packageA},
		{From: packageSelf, To: packageSelf},
	}
	if got := selection.Edges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Edges = %#v, want %#v", got, want)
	} else if cap(got) > MaxRows {
		t.Fatalf("Edges capacity = %d, want <= %d", cap(got), MaxRows)
	}

	candidates[0] = Candidate{From: packageB, To: packageA}
	exposed := selection.Edges()
	exposed[0] = Edge{From: packageA, To: packageC}
	if got := selection.Edges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("caller mutation changed selection: %#v", got)
	}

	maxCandidates := make([]Candidate, MaxRows)
	for index := range maxCandidates {
		maxCandidates[index] = Candidate{From: packageA, To: packageB}
	}
	maxSelection, err := New(Input{Graph: graph, Candidates: maxCandidates})
	if err != nil {
		t.Fatalf("New maximum rows: %v", err)
	}
	if got := maxSelection.Edges(); len(got) != MaxRows || cap(got) > MaxRows {
		t.Fatalf("maximum Edges len/cap = %d/%d, want %d/<=%d", len(got), cap(got), MaxRows, MaxRows)
	}
}

func TestSelectionPreservesNilAndEmptyShape(t *testing.T) {
	nilSelection, err := New(Input{})
	if err != nil {
		t.Fatalf("New nil: %v", err)
	}
	if got := nilSelection.Edges(); got != nil {
		t.Fatalf("nil Edges = %#v, want nil", got)
	}

	emptySelection, err := New(Input{Candidates: []Candidate{}})
	if err != nil {
		t.Fatalf("New empty: %v", err)
	}
	if got := emptySelection.Edges(); got == nil || len(got) != 0 {
		t.Fatalf("empty Edges = %#v, want non-nil empty", got)
	}

	if got := (Selection{}).Edges(); got != nil {
		t.Fatalf("zero Selection Edges = %#v, want nil", got)
	}
}

func TestSelectionRejectsUnavailableExactAuthority(t *testing.T) {
	graph := newSelectionTestGraph(t, []gofacts.Edge{
		{From: packageA, To: packageB},
		{From: packageB, To: packageC},
		{From: packageSelf, To: packageSelf},
	})
	tests := []struct {
		name      string
		candidate Candidate
	}{
		{name: "missing from", candidate: Candidate{To: packageB}},
		{name: "missing to", candidate: Candidate{From: packageA}},
		{name: "absent direct pair", candidate: Candidate{From: packageA, To: packageC}},
		{name: "reverse only", candidate: Candidate{From: packageB, To: packageA}},
		{name: "external from", candidate: Candidate{From: "example.net/external", To: packageB}},
		{name: "test-only package", candidate: Candidate{From: packageA + "_test", To: packageB}},
		{name: "alias-like endpoint", candidate: Candidate{From: "example.com/repo/./a", To: packageB}},
		{name: "absolute endpoint", candidate: Candidate{From: "/private/repo/a", To: packageB}},
		{name: "unauthorized self edge", candidate: Candidate{From: packageA, To: packageA}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection, err := New(Input{
				Graph: graph,
				Candidates: []Candidate{
					{From: packageA, To: packageB},
					test.candidate,
				},
			})
			if !errors.Is(err, errUnauthorized) {
				t.Fatalf("New error = %v, want unauthorized", err)
			}
			if got := selection.Edges(); got != nil {
				t.Fatalf("failed selection exposed prefix: %#v", got)
			}
			if test.candidate.From != "" && strings.Contains(err.Error(), test.candidate.From) ||
				test.candidate.To != "" && strings.Contains(err.Error(), test.candidate.To) ||
				strings.Contains(err.Error(), "/private/") {
				t.Fatalf("error echoed caller scalar: %v", err)
			}
		})
	}
}

func TestSelectionRequiresGraphAuthorization(t *testing.T) {
	emptyGraph := newSelectionTestGraph(t, nil)
	_, err := New(Input{
		Graph:      emptyGraph,
		Candidates: []Candidate{{From: packageA, To: packageB}},
	})
	if !errors.Is(err, errUnauthorized) {
		t.Fatalf("empty graph error = %v, want unauthorized", err)
	}

	_, err = New(Input{
		Candidates: []Candidate{{From: packageA, To: packageB}},
	})
	if !errors.Is(err, errUnauthorized) {
		t.Fatalf("zero graph error = %v, want unauthorized", err)
	}
}

func TestSelectionPreflightBudgetPrecedence(t *testing.T) {
	oversized := strings.Repeat("x", MaxEndpointBytes+1)

	t.Run("raw count before scalar", func(t *testing.T) {
		candidates := make([]Candidate, MaxRows+1)
		candidates[0].From = oversized
		_, err := New(Input{Candidates: candidates})
		if !errors.Is(err, errRawBounds) {
			t.Fatalf("New error = %v, want raw bounds", err)
		}
	})

	t.Run("individual endpoint before graph", func(t *testing.T) {
		_, err := New(Input{Candidates: []Candidate{{From: oversized}}})
		if !errors.Is(err, errEndpointBounds) {
			t.Fatalf("New error = %v, want endpoint bounds", err)
		}
	})

	t.Run("aggregate endpoint budget", func(t *testing.T) {
		endpoint := strings.Repeat("x", MaxEndpointBytes)
		candidates := make([]Candidate, MaxAggregateEndpointBytes/(2*MaxEndpointBytes)+1)
		for index := range candidates {
			candidates[index] = Candidate{From: endpoint, To: endpoint}
		}
		_, err := New(Input{Candidates: candidates})
		if !errors.Is(err, errAggregateBounds) {
			t.Fatalf("New error = %v, want aggregate bounds", err)
		}
	})

	t.Run("individual endpoint precedes aggregate remaining", func(t *testing.T) {
		endpoint := strings.Repeat("x", MaxEndpointBytes)
		candidates := make([]Candidate, MaxAggregateEndpointBytes/(2*MaxEndpointBytes)+1)
		for index := range candidates[:len(candidates)-1] {
			candidates[index] = Candidate{From: endpoint, To: endpoint}
		}
		candidates[len(candidates)-1] = Candidate{From: "x", To: oversized}
		_, err := New(Input{Candidates: candidates})
		if !errors.Is(err, errEndpointBounds) {
			t.Fatalf("New error = %v, want endpoint bounds", err)
		}
	})

	t.Run("exact aggregate bound reaches authority", func(t *testing.T) {
		endpoint := strings.Repeat("x", MaxEndpointBytes)
		candidates := make([]Candidate, MaxAggregateEndpointBytes/(2*MaxEndpointBytes))
		for index := range candidates {
			candidates[index] = Candidate{From: endpoint, To: endpoint}
		}
		_, err := New(Input{Candidates: candidates})
		if !errors.Is(err, errUnauthorized) {
			t.Fatalf("New error = %v, want authority lookup", err)
		}
	})
}

func TestOversizedEndpointPreflightDoesNotAllocate(t *testing.T) {
	input := Input{Candidates: []Candidate{{
		From: strings.Repeat("x", MaxEndpointBytes+1),
	}}}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, benchmarkSelectionError = New(input)
	}); allocations != 0 {
		t.Fatalf("oversized preflight allocations = %v, want 0", allocations)
	}
	if !errors.Is(benchmarkSelectionError, errEndpointBounds) {
		t.Fatalf("New error = %v, want endpoint bounds", benchmarkSelectionError)
	}
}

func BenchmarkNewPreflightOversizedEndpoint(b *testing.B) {
	input := Input{Candidates: []Candidate{{
		From: strings.Repeat("x", MaxEndpointBytes+1),
	}}}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, benchmarkSelectionError = New(input)
	}
}

func TestWorkspaceEdgeSelectionProductionDependenciesStayNeutral(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	directory := filepath.Dir(testFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filename := filepath.Join(directory, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", entry.Name(), err)
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("Unquote(%s): %v", imported.Path.Value, err)
			}
			if strings.HasPrefix(importPath, "github.com/dvordrova/repomap/internal/") &&
				importPath != "github.com/dvordrova/repomap/internal/workspacegraph" {
				t.Fatalf("production dependency %q is outside the neutral contract", importPath)
			}
		}
	}
}

func newSelectionTestGraph(t *testing.T, edges []gofacts.Edge) workspacegraph.Graph {
	t.Helper()
	const repositoryRoot = "/definitely-not-present/workspace-edge-selection"
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: repositoryRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	packageFact := func(canonicalPath, name, directory string) gofacts.PackageFact {
		return gofacts.PackageFact{
			CanonicalPath:     canonicalPath,
			Name:              name,
			ModuleID:          "root-id",
			ModulePath:        "example.com/repo",
			PackageDir:        directory,
			ModuleRelativeDir: directory,
		}
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts: gofacts.Facts{
			Modules: []gofacts.ModuleFact{{
				ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
			}},
			Packages: []gofacts.PackageFact{
				packageFact(packageA, "a", "a"),
				packageFact(packageB, "b", "b"),
				packageFact(packageC, "c", "c"),
				packageFact(packageSelf, "self", "self"),
			},
			InternalEdges: append([]gofacts.Edge(nil), edges...),
		},
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	return graph
}

func TestSelectionErrorsDoNotExposeAbsoluteRoots(t *testing.T) {
	root := "/definitely-not-present/private-workspace"
	_, err := New(Input{Candidates: []Candidate{{From: root, To: packageB}}})
	if err == nil {
		t.Fatal("New unexpectedly succeeded")
	}
	if strings.Contains(fmt.Sprint(err), root) {
		t.Fatalf("error exposed absolute root: %v", err)
	}
}
