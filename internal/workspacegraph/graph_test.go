package workspacegraph

import (
	"crypto/sha256"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestNewBuildsDeterministicAuthorizedGraph(t *testing.T) {
	snapshot := testSnapshot(t, "", []string{
		"main.go",
		"internal/core/core.go",
		"tools/cmd/tool/main.go",
	})
	facts := representativeFacts()

	graph, err := New(Input{Snapshot: snapshot, GoFacts: facts})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	wantModules := []Module{
		{ID: "root-id", Path: "example.com/repo", Dir: ".", Main: true, GoMod: "go.mod"},
		{ID: "tools-id", Path: "example.com/repo/tools", Dir: "tools", Main: true, GoMod: "tools/go.mod"},
	}
	if got := graph.Modules(); !reflect.DeepEqual(got, wantModules) {
		t.Fatalf("Modules = %#v, want %#v", got, wantModules)
	}
	wantPackages := []Package{
		{
			CanonicalPath: "example.com/repo/cmd/app", Name: "main",
			ModuleID: "root-id", ModulePath: "example.com/repo",
			Dir: ".", ModuleRelativeDir: ".",
			Files: []File{
				{Path: "generated.go", Openable: false},
				{Path: "main.go", Openable: true},
			},
		},
		{
			CanonicalPath: "example.com/repo/internal/core", Name: "core",
			ModuleID: "root-id", ModulePath: "example.com/repo",
			Dir: "internal/core", ModuleRelativeDir: "internal/core",
			Files: []File{{Path: "internal/core/core.go", Openable: true}},
		},
		{
			CanonicalPath: "example.com/repo/tools/cmd/tool", Name: "main",
			ModuleID: "tools-id", ModulePath: "example.com/repo/tools",
			Dir: "tools/cmd/tool", ModuleRelativeDir: "cmd/tool",
			Files: []File{{Path: "tools/cmd/tool/main.go", Openable: true}},
		},
	}
	if got := graph.Packages(); !reflect.DeepEqual(got, wantPackages) {
		t.Fatalf("Packages = %#v, want %#v", got, wantPackages)
	}
	wantEdges := []Edge{
		{FromPackage: "example.com/repo/cmd/app", ToPackage: "example.com/repo/internal/core"},
		{FromPackage: "example.com/repo/tools/cmd/tool", ToPackage: "example.com/repo/internal/core"},
	}
	if got := graph.Edges(); !reflect.DeepEqual(got, wantEdges) {
		t.Fatalf("Edges = %#v, want %#v", got, wantEdges)
	}

	permuted := permuteFacts(facts)
	permutedGraph, err := New(Input{Snapshot: snapshot, GoFacts: permuted})
	if err != nil {
		t.Fatalf("New(permuted): %v", err)
	}
	if !reflect.DeepEqual(graph.Modules(), permutedGraph.Modules()) ||
		!reflect.DeepEqual(graph.Packages(), permutedGraph.Packages()) ||
		!reflect.DeepEqual(graph.Edges(), permutedGraph.Edges()) {
		t.Fatalf(
			"permuted graph differs:\nmodules %#v\npackages %#v\nedges %#v",
			permutedGraph.Modules(),
			permutedGraph.Packages(),
			permutedGraph.Edges(),
		)
	}
}

func TestNewFiltersOutsideFactsAndUnknownOrExternalEdges(t *testing.T) {
	snapshot := testSnapshot(t, "", []string{"main.go", "internal/core/core.go"})
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/repo/app", Name: "app",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: ".", ModuleRelativeDir: ".",
				Files: []string{
					"main.go",
					"main.go",
					"../escape.go",
					"/absolute.go",
					"nested/not-a-package-file.go",
				},
			},
			{
				CanonicalPath: "example.com/repo/core", Name: "core",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "internal/core", ModuleRelativeDir: "internal/core",
				Files: []string{"internal/core/core.go"},
			},
			{
				CanonicalPath: "example.com/repo/outside", Name: "outside",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "../outside", ModuleRelativeDir: "../outside",
			},
			{
				CanonicalPath: "bad package identity", Name: "bad",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "bad", ModuleRelativeDir: "bad",
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/repo/app", To: "example.com/repo/core"},
			{From: "example.com/repo/app", To: "fmt"},
			{From: "example.com/repo/app", To: "example.com/repo/outside"},
			{From: "bad edge identity", To: "example.com/repo/core"},
		},
	}
	graph, err := New(Input{Snapshot: snapshot, GoFacts: facts})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := graph.Packages(); len(got) != 2 {
		t.Fatalf("Packages = %#v, want two retained packages", got)
	}
	app, ok := graph.Package("example.com/repo/app")
	if !ok {
		t.Fatal("app package missing")
	}
	if want := []File{{Path: "main.go", Openable: true}}; !reflect.DeepEqual(app.Files, want) {
		t.Fatalf("app files = %#v, want %#v", app.Files, want)
	}
	if got := graph.Edges(); !reflect.DeepEqual(got, []Edge{{
		FromPackage: "example.com/repo/app",
		ToPackage:   "example.com/repo/core",
	}}) {
		t.Fatalf("Edges = %#v", got)
	}
}

func TestNewKeepsCompositePackagesAndFailsClosedOnAmbiguousCanonicalPath(t *testing.T) {
	facts := compositePackageFacts()
	graph, err := New(Input{
		Snapshot: testSnapshot(t, "", nil),
		GoFacts:  facts,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := len(graph.Packages()); got != 4 {
		t.Fatalf("Packages = %d, want 4", got)
	}
	if got := graph.Edges(); !reflect.DeepEqual(got, []Edge{{
		FromPackage: "example.com/root/app",
		ToPackage:   "example.com/root/core",
	}}) {
		t.Fatalf("Edges = %#v", got)
	}

	const sharedPath = "example.com/shared"
	if pkg, ok := graph.Package(sharedPath); ok {
		t.Fatalf("ambiguous canonical lookup returned %#v", pkg)
	}
	first, ok := graph.PackageInModule(sharedPath, "fixture-a")
	if !ok || first.ModuleID != "fixture-a" || first.Dir != "fixtures/a" {
		t.Fatalf("first composite lookup = %#v, %v", first, ok)
	}
	second, ok := graph.PackageInModule(sharedPath, "fixture-b")
	if !ok || second.ModuleID != "fixture-b" || second.Dir != "fixtures/b" {
		t.Fatalf("second composite lookup = %#v, %v", second, ok)
	}
	if _, ok := graph.PackageInModule(sharedPath, "missing-module"); ok {
		t.Fatal("missing composite lookup succeeded")
	}
	first.Name = "mutated"
	firstAgain, _ := graph.PackageInModule(sharedPath, "fixture-a")
	if firstAgain.Name != "main" {
		t.Fatalf("composite lookup leaked mutation: %#v", firstAgain)
	}

	permuted, err := New(Input{
		Snapshot: testSnapshot(t, "", nil),
		GoFacts:  permuteFacts(facts),
	})
	if err != nil {
		t.Fatalf("New(permuted): %v", err)
	}
	if !publicGraphsEqual(graph, permuted) {
		t.Fatalf("permuted composite graph differs: %#v", permuted.Packages())
	}
}

func TestNewRejectsEdgesWithAmbiguousPackageIdentity(t *testing.T) {
	const (
		ambiguous = "example.com/shared"
		unique    = "example.com/root/app"
	)
	tests := []struct {
		name string
		edge gofacts.Edge
	}{
		{name: "ambiguous source", edge: gofacts.Edge{From: ambiguous, To: unique}},
		{name: "ambiguous target", edge: gofacts.Edge{From: unique, To: ambiguous}},
		{name: "ambiguous with unknown peer", edge: gofacts.Edge{From: ambiguous, To: "example.net/external"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := compositePackageFacts()
			facts.InternalEdges = append(facts.InternalEdges, test.edge)
			graph, err := New(Input{
				Snapshot: testSnapshot(t, "", nil),
				GoFacts:  facts,
			})
			if err == nil {
				t.Fatal("New unexpectedly accepted an ambiguous edge endpoint")
			}
			if graph.Modules() != nil || graph.Packages() != nil || graph.Edges() != nil {
				t.Fatalf("failed graph exposed partial state: %#v", graph)
			}
			if strings.Contains(err.Error(), ambiguous) {
				t.Fatalf("error exposed ambiguous package identity: %v", err)
			}
		})
	}
}

func TestNewRejectsConflictingOrInconsistentExactFacts(t *testing.T) {
	snapshot := testSnapshot(t, "", nil)
	baseModule := gofacts.ModuleFact{
		ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
	}
	basePackage := gofacts.PackageFact{
		CanonicalPath: "example.com/repo/app", Name: "app",
		ModuleID: "root-id", ModulePath: "example.com/repo",
		PackageDir: ".", ModuleRelativeDir: ".",
	}
	tests := []struct {
		name  string
		facts gofacts.Facts
	}{
		{
			name: "module id conflict",
			facts: gofacts.Facts{Modules: []gofacts.ModuleFact{
				baseModule,
				{ID: "root-id", ModulePath: "example.com/other", ModuleDir: "."},
			}},
		},
		{
			name: "module location conflict",
			facts: gofacts.Facts{Modules: []gofacts.ModuleFact{
				baseModule,
				{ID: "other-id", ModulePath: "example.com/repo", ModuleDir: "."},
			}},
		},
		{
			name: "package identity conflict",
			facts: gofacts.Facts{
				Modules: []gofacts.ModuleFact{baseModule},
				Packages: []gofacts.PackageFact{
					basePackage,
					{
						CanonicalPath: "example.com/repo/app", Name: "other",
						ModuleID: "root-id", ModulePath: "example.com/repo",
						PackageDir: ".", ModuleRelativeDir: ".",
					},
				},
			},
		},
		{
			name: "unknown module owner",
			facts: gofacts.Facts{
				Modules: []gofacts.ModuleFact{baseModule},
				Packages: []gofacts.PackageFact{{
					CanonicalPath: "example.com/repo/app", Name: "app",
					ModuleID: "other-id", ModulePath: "example.com/other",
					PackageDir: ".", ModuleRelativeDir: ".",
				}},
			},
		},
		{
			name: "inconsistent nested module directory",
			facts: gofacts.Facts{
				Modules: []gofacts.ModuleFact{{
					ID: "nested-id", ModulePath: "example.com/repo/tools", ModuleDir: "tools",
				}},
				Packages: []gofacts.PackageFact{{
					CanonicalPath: "example.com/repo/tools/app", Name: "app",
					ModuleID: "nested-id", ModulePath: "example.com/repo/tools",
					PackageDir: "other/app", ModuleRelativeDir: "app",
				}},
			},
		},
		{
			name: "malformed module identity",
			facts: gofacts.Facts{Modules: []gofacts.ModuleFact{{
				ID: "root-id", ModulePath: "../repo", ModuleDir: ".",
			}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(Input{Snapshot: snapshot, GoFacts: test.facts}); err == nil {
				t.Fatal("New unexpectedly succeeded")
			}
		})
	}
}

func TestNewEnforcesRawBudgetsBeforeValidation(t *testing.T) {
	snapshot := testSnapshot(t, "", nil)
	longScalar := strings.Repeat("x", maxScalarBytes+1)
	withinScalar := strings.Repeat("x", maxScalarBytes)

	aggregateFiles := make([]gofacts.PackageFact, 5)
	for index := range aggregateFiles {
		aggregateFiles[index].Files = make([]string, 4001)
	}
	aggregateScalars := make([]gofacts.ModuleFact, maxModules)
	for index := range aggregateScalars {
		aggregateScalars[index] = gofacts.ModuleFact{
			ID: withinScalar, ModulePath: withinScalar,
			ModuleDir: withinScalar, GoMod: withinScalar,
		}
	}
	tests := []struct {
		name  string
		facts gofacts.Facts
	}{
		{name: "modules", facts: gofacts.Facts{Modules: make([]gofacts.ModuleFact, maxModules+1)}},
		{name: "packages", facts: gofacts.Facts{Packages: make([]gofacts.PackageFact, maxPackages+1)}},
		{name: "edges", facts: gofacts.Facts{InternalEdges: make([]gofacts.Edge, MaxExactEdges+1)}},
		{name: "files per package", facts: gofacts.Facts{Packages: []gofacts.PackageFact{{
			Files: make([]string, maxFilesPerPackage+1),
		}}}},
		{name: "aggregate files", facts: gofacts.Facts{Packages: aggregateFiles}},
		{name: "scalar bytes", facts: gofacts.Facts{Modules: []gofacts.ModuleFact{{
			ID: "\x00" + longScalar,
		}}}},
		{name: "aggregate scalar bytes", facts: gofacts.Facts{Modules: aggregateScalars}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(Input{Snapshot: snapshot, GoFacts: test.facts})
			if err == nil {
				t.Fatal("New unexpectedly succeeded")
			}
			if strings.Contains(err.Error(), longScalar[:128]) || strings.Contains(err.Error(), "\x00") {
				t.Fatalf("error echoed analyzer scalar: %q", err)
			}
		})
	}
}

func TestNewRequiresSnapshotAndDoesNotEchoPrivateInput(t *testing.T) {
	privateValue := "/private/workspace-owner/secret"
	_, err := New(Input{GoFacts: gofacts.Facts{Modules: []gofacts.ModuleFact{{
		ID: "id", ModulePath: privateValue, ModuleDir: ".",
	}}}})
	if err == nil {
		t.Fatal("New unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), privateValue) {
		t.Fatalf("error exposed private input: %q", err)
	}
}

func TestGraphIgnoresEditorialFactsAndChangesForExactFacts(t *testing.T) {
	snapshot := testSnapshot(t, "", []string{"main.go"})
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
			DisplayName: "Repository", Warnings: []string{"old warning"},
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/repo/app", Name: "app",
			ModuleID: "root-id", ModulePath: "example.com/repo",
			PackageDir: ".", ModuleRelativeDir: ".",
			DisplayPath: "Application", Locality: "local",
			Files: []string{"main.go"},
		}},
		ModuleSummaries: []gofacts.ModuleSummary{{RoleGuess: "service"}},
		OrientationCandidates: []gofacts.OrientationCandidate{{
			Name: "Start", Why: "editorial",
		}},
		Warnings: []string{"top-level warning"},
	}
	before, err := New(Input{Snapshot: snapshot, GoFacts: facts})
	if err != nil {
		t.Fatalf("New(before): %v", err)
	}

	editorial := facts
	editorial.Modules = append([]gofacts.ModuleFact(nil), facts.Modules...)
	editorial.Modules[0].DisplayName = strings.Repeat("display", 10_000)
	editorial.Modules[0].Warnings = []string{strings.Repeat("warning", 10_000)}
	editorial.Packages = append([]gofacts.PackageFact(nil), facts.Packages...)
	editorial.Packages[0].DisplayPath = "Changed presentation label"
	editorial.Packages[0].Locality = "changed presentation locality"
	editorial.ModuleSummaries = []gofacts.ModuleSummary{{RoleGuess: "different role"}}
	editorial.OrientationCandidates = []gofacts.OrientationCandidate{{
		Name: "Different", Why: strings.Repeat("model prose", 10_000),
	}}
	editorial.Warnings = []string{strings.Repeat("different warning", 10_000)}
	afterEditorial, err := New(Input{Snapshot: snapshot, GoFacts: editorial})
	if err != nil {
		t.Fatalf("New(editorial): %v", err)
	}
	if !publicGraphsEqual(before, afterEditorial) {
		t.Fatalf(
			"editorial fields changed graph:\nbefore %#v %#v %#v\nafter %#v %#v %#v",
			before.Modules(), before.Packages(), before.Edges(),
			afterEditorial.Modules(), afterEditorial.Packages(), afterEditorial.Edges(),
		)
	}

	exact := facts
	exact.Packages = append([]gofacts.PackageFact(nil), facts.Packages...)
	exact.Packages[0].Name = "changed"
	afterExact, err := New(Input{Snapshot: snapshot, GoFacts: exact})
	if err != nil {
		t.Fatalf("New(exact): %v", err)
	}
	if publicGraphsEqual(before, afterExact) {
		t.Fatal("exact package name change did not change graph")
	}
}

func TestGraphAccessorsAndQueriesAreDefensive(t *testing.T) {
	snapshot := testSnapshot(t, "", []string{"main.go", "internal/core/core.go", "tools/cmd/tool/main.go"})
	graph, err := New(Input{Snapshot: snapshot, GoFacts: representativeFacts()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	modules := graph.Modules()
	modules[0].Path = "mutated"
	packages := graph.Packages()
	packages[0].Name = "mutated"
	packages[0].Files[0].Path = "mutated.go"
	edges := graph.Edges()
	edges[0].FromPackage = "mutated"

	module, ok := graph.Module("root-id", "example.com/repo", ".")
	if !ok || module.Path != "example.com/repo" {
		t.Fatalf("Module after mutation = %#v, %v", module, ok)
	}
	pkg, ok := graph.Package("example.com/repo/cmd/app")
	if !ok || pkg.Name != "main" || pkg.Files[0].Path != "generated.go" {
		t.Fatalf("Package after mutation = %#v, %v", pkg, ok)
	}
	pkg.Files[0].Path = "second-mutation.go"
	again, _ := graph.Package("example.com/repo/cmd/app")
	if again.Files[0].Path != "generated.go" {
		t.Fatalf("Package lookup leaked mutation: %#v", again)
	}
	edge, ok := graph.Edge("example.com/repo/cmd/app", "example.com/repo/internal/core")
	if !ok || edge.FromPackage != "example.com/repo/cmd/app" {
		t.Fatalf("Edge after mutation = %#v, %v", edge, ok)
	}

	oversized := strings.Repeat("x", maxScalarBytes+1)
	if _, ok := graph.Module(oversized, "example.com/repo", "."); ok {
		t.Fatal("oversized module query succeeded")
	}
	if _, ok := graph.Package(oversized); ok {
		t.Fatal("oversized package query succeeded")
	}
	if _, ok := graph.PackageInModule("example.com/repo/cmd/app", oversized); ok {
		t.Fatal("oversized composite package query succeeded")
	}
	if _, ok := graph.Edge(oversized, "example.com/repo/internal/core"); ok {
		t.Fatal("oversized edge query succeeded")
	}
}

func TestWorkspaceGraphProductionDependenciesStayNeutral(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	directory := filepath.Dir(testFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	allowedInternal := map[string]bool{
		"github.com/dvordrova/repomap/internal/gofacts":           true,
		"github.com/dvordrova/repomap/internal/workspacesnapshot": true,
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
				!allowedInternal[importPath] {
				t.Fatalf("production dependency %q is outside the neutral contract", importPath)
			}
		}
	}
}

func representativeFacts() gofacts.Facts {
	root := gofacts.ModuleFact{
		ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
		Main: true, GoMod: "go.mod", DisplayName: ".",
	}
	tools := gofacts.ModuleFact{
		ID: "tools-id", ModulePath: "example.com/repo/tools", ModuleDir: "tools",
		Main: true, GoMod: "tools/go.mod", DisplayName: "tools",
	}
	core := gofacts.PackageFact{
		CanonicalPath: "example.com/repo/internal/core", Name: "core",
		ModuleID: "root-id", ModulePath: "example.com/repo",
		PackageDir: "internal/core", ModuleRelativeDir: "internal/core",
		DisplayPath: "internal/core", Locality: "local",
		Files: []string{"internal/core/core.go", "internal/core/core.go"},
	}
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			tools,
			root,
			{
				ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
				Main: true, GoMod: "go.mod", DisplayName: "ignored duplicate display",
			},
		},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/repo/tools/cmd/tool", Name: "main",
				ModuleID: "tools-id", ModulePath: "example.com/repo/tools",
				PackageDir: "tools/cmd/tool", ModuleRelativeDir: "cmd/tool",
				DisplayPath: "cmd/tool", Locality: "local",
				Files: []string{"tools/cmd/tool/main.go"},
			},
			core,
			{
				CanonicalPath: "example.com/repo/cmd/app", Name: "main",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: ".", ModuleRelativeDir: ".",
				DisplayPath: "main", Locality: "local",
				Files: []string{"main.go", "generated.go"},
			},
			{
				CanonicalPath: "example.com/repo/internal/core", Name: "core",
				ModuleID: "root-id", ModulePath: "example.com/repo",
				PackageDir: "internal/core", ModuleRelativeDir: "internal/core",
				DisplayPath: "different ignored display", Locality: "different ignored locality",
				Files: []string{"internal/core/core.go"},
			},
		},
		InternalEdges: []gofacts.Edge{
			{From: "example.com/repo/tools/cmd/tool", To: "example.com/repo/internal/core"},
			{From: "example.com/repo/cmd/app", To: "fmt"},
			{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
			{From: "example.com/repo/cmd/app", To: "example.com/repo/internal/core"},
			{From: "example.com/repo/missing", To: "example.com/repo/internal/core"},
		},
	}
}

func compositePackageFacts() gofacts.Facts {
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{ID: "root-id", ModulePath: "example.com/root", ModuleDir: "."},
			{ID: "fixture-a", ModulePath: "example.com/shared", ModuleDir: "fixtures/a"},
			{ID: "fixture-b", ModulePath: "example.com/shared", ModuleDir: "fixtures/b"},
		},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/shared", Name: "main",
				ModuleID: "fixture-a", ModulePath: "example.com/shared",
				PackageDir: "fixtures/a", ModuleRelativeDir: ".",
			},
			{
				CanonicalPath: "example.com/shared", Name: "sum",
				ModuleID: "fixture-b", ModulePath: "example.com/shared",
				PackageDir: "fixtures/b", ModuleRelativeDir: ".",
			},
			{
				CanonicalPath: "example.com/root/app", Name: "app",
				ModuleID: "root-id", ModulePath: "example.com/root",
				PackageDir: "app", ModuleRelativeDir: "app",
			},
			{
				CanonicalPath: "example.com/root/core", Name: "core",
				ModuleID: "root-id", ModulePath: "example.com/root",
				PackageDir: "core", ModuleRelativeDir: "core",
			},
		},
		InternalEdges: []gofacts.Edge{{
			From: "example.com/root/app",
			To:   "example.com/root/core",
		}},
	}
}

func permuteFacts(facts gofacts.Facts) gofacts.Facts {
	result := facts
	result.Modules = reverseCopy(facts.Modules)
	result.Packages = reverseCopy(facts.Packages)
	for index := range result.Packages {
		result.Packages[index].Files = reverseCopy(result.Packages[index].Files)
	}
	result.InternalEdges = reverseCopy(facts.InternalEdges)
	return result
}

func reverseCopy[T any](values []T) []T {
	result := append([]T(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func publicGraphsEqual(left, right Graph) bool {
	return reflect.DeepEqual(left.Modules(), right.Modules()) &&
		reflect.DeepEqual(left.Packages(), right.Packages()) &&
		reflect.DeepEqual(left.Edges(), right.Edges())
}

func testSnapshot(t *testing.T, analysisSubdir string, allowedPaths []string) workspacesnapshot.Snapshot {
	t.Helper()
	repositoryRoot := filepath.Clean("/workspacegraph-test")
	analysisRoot := repositoryRoot
	if analysisSubdir != "" {
		analysisRoot = filepath.Join(repositoryRoot, filepath.FromSlash(analysisSubdir))
	}
	captured := make([]freshness.CapturedInput, 0, len(allowedPaths))
	for _, allowedPath := range allowedPaths {
		repositoryPath := allowedPath
		if analysisSubdir != "" {
			repositoryPath = path.Join(analysisSubdir, allowedPath)
		}
		id := sha256.Sum256([]byte("id:" + repositoryPath))
		content := sha256.Sum256([]byte("content:" + repositoryPath))
		captured = append(captured, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", id),
			Path:          repositoryPath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", content),
			Stages:        []string{"workspace_graph_test"},
		})
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: analysisRoot,
		Repository: freshness.RepositoryState{
			Version:  freshness.RepositoryStateVersion,
			Identity: repositoryRoot,
			Head:     strings.Repeat("a", 40),
			Dirty:    []freshness.DirtyFile{},
		},
		CapturedInputs: captured,
		AllowedPaths:   append([]string(nil), allowedPaths...),
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	return snapshot
}
