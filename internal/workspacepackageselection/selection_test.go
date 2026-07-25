package workspacepackageselection

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
	packageRoot = "example.com/repo"
	packageTool = "example.com/repo/tools/cmd/tool"
)

var benchmarkPackageSelectionError error

func TestSelectionPreservesExactOrderDuplicatesAndAuthority(t *testing.T) {
	facts := defaultSelectionFacts()
	graph := newPackageSelectionTestGraph(
		t,
		"/definitely-not-present/workspace-package-selection",
		"/definitely-not-present/workspace-package-selection",
		facts,
	)
	candidates := []Candidate{
		candidateFromFact(facts.Packages[2]),
		candidateFromFact(facts.Packages[0]),
		candidateFromFact(facts.Packages[2]),
		candidateFromFact(facts.Packages[3]),
	}
	selection, err := New(Input{Graph: graph, Candidates: candidates})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := []Package{
		packageFromCandidate(candidates[0]),
		packageFromCandidate(candidates[1]),
		packageFromCandidate(candidates[2]),
		packageFromCandidate(candidates[3]),
	}
	if got := selection.Packages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Packages = %#v, want %#v", got, want)
	} else if cap(got) > MaxRows {
		t.Fatalf("Packages capacity = %d, want <= %d", cap(got), MaxRows)
	}

	candidates[0].Name = "changed"
	exposed := selection.Packages()
	exposed[0].Name = "changed"
	if got := selection.Packages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("caller mutation changed selection: %#v", got)
	}

	maxCandidates := make([]Candidate, MaxRows)
	for index := range maxCandidates {
		maxCandidates[index] = candidateFromFact(facts.Packages[0])
	}
	maxSelection, err := New(Input{Graph: graph, Candidates: maxCandidates})
	if err != nil {
		t.Fatalf("New maximum rows: %v", err)
	}
	if got := maxSelection.Packages(); len(got) != MaxRows || cap(got) > MaxRows {
		t.Fatalf(
			"maximum Packages len/cap = %d/%d, want %d/<=%d",
			len(got),
			cap(got),
			MaxRows,
			MaxRows,
		)
	}
}

func TestSelectionPreservesNilAndEmptyShape(t *testing.T) {
	nilSelection, err := New(Input{})
	if err != nil {
		t.Fatalf("New nil: %v", err)
	}
	if got := nilSelection.Packages(); got != nil {
		t.Fatalf("nil Packages = %#v, want nil", got)
	}

	emptySelection, err := New(Input{Candidates: []Candidate{}})
	if err != nil {
		t.Fatalf("New empty: %v", err)
	}
	if got := emptySelection.Packages(); got == nil || len(got) != 0 {
		t.Fatalf("empty Packages = %#v, want non-nil empty", got)
	}

	if got := (Selection{}).Packages(); got != nil {
		t.Fatalf("zero Selection Packages = %#v, want nil", got)
	}
}

func TestSelectionSupportsWorkspaceLayouts(t *testing.T) {
	tests := []struct {
		name           string
		repositoryRoot string
		analysisRoot   string
		facts          gofacts.Facts
	}{
		{
			name:           "repository root",
			repositoryRoot: "/definitely-not-present/package-root",
			analysisRoot:   "/definitely-not-present/package-root",
			facts: gofacts.Facts{
				Modules: []gofacts.ModuleFact{{
					ID: "root-id", ModulePath: packageRoot, ModuleDir: ".", Main: true,
				}},
				Packages: []gofacts.PackageFact{{
					CanonicalPath: packageRoot, Name: "repo",
					ModuleID: "root-id", ModulePath: packageRoot,
					PackageDir: ".", ModuleRelativeDir: ".",
				}},
			},
		},
		{
			name:           "nested module",
			repositoryRoot: "/definitely-not-present/package-nested",
			analysisRoot:   "/definitely-not-present/package-nested",
			facts: gofacts.Facts{
				Modules: []gofacts.ModuleFact{
					{ID: "root-id", ModulePath: packageRoot, ModuleDir: ".", Main: true},
					{
						ID: "tools-id", ModulePath: "example.com/repo/tools",
						ModuleDir: "tools",
					},
				},
				Packages: []gofacts.PackageFact{{
					CanonicalPath: packageTool, Name: "main",
					ModuleID: "tools-id", ModulePath: "example.com/repo/tools",
					PackageDir: "tools/cmd/tool", ModuleRelativeDir: "cmd/tool",
				}},
			},
		},
		{
			name:           "subdirectory analysis",
			repositoryRoot: "/definitely-not-present/package-subdirectory",
			analysisRoot:   "/definitely-not-present/package-subdirectory/service",
			facts: gofacts.Facts{
				Modules: []gofacts.ModuleFact{{
					ID: "service-id", ModulePath: "example.com/service",
					ModuleDir: ".", Main: true,
				}},
				Packages: []gofacts.PackageFact{{
					CanonicalPath: "example.com/service/internal/core", Name: "core",
					ModuleID: "service-id", ModulePath: "example.com/service",
					PackageDir:        "internal/core",
					ModuleRelativeDir: "internal/core",
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := newPackageSelectionTestGraph(
				t,
				test.repositoryRoot,
				test.analysisRoot,
				test.facts,
			)
			candidate := candidateFromFact(test.facts.Packages[0])
			selection, err := New(Input{
				Graph:      graph,
				Candidates: []Candidate{candidate},
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := selection.Packages(); !reflect.DeepEqual(
				got,
				[]Package{packageFromCandidate(candidate)},
			) {
				t.Fatalf("Packages = %#v", got)
			}
		})
	}
}

func TestSelectionRejectsUnavailableAndConflictingAuthority(t *testing.T) {
	facts := defaultSelectionFacts()
	graph := newPackageSelectionTestGraph(
		t,
		"/definitely-not-present/package-authority",
		"/definitely-not-present/package-authority",
		facts,
	)
	valid := candidateFromFact(facts.Packages[0])
	tests := []struct {
		name   string
		change func(*Candidate)
	}{
		{
			name: "missing package",
			change: func(candidate *Candidate) {
				candidate.CanonicalPath = "example.com/repo/missing-private-caller"
			},
		},
		{
			name: "external package",
			change: func(candidate *Candidate) {
				candidate.CanonicalPath = "example.net/external-private-caller"
			},
		},
		{
			name: "test-only package",
			change: func(candidate *Candidate) {
				candidate.CanonicalPath = packageA + "_test"
			},
		},
		{
			name: "alias-like package",
			change: func(candidate *Candidate) {
				candidate.CanonicalPath = "example.com/repo/./a"
			},
		},
		{
			name: "absolute package",
			change: func(candidate *Candidate) {
				candidate.CanonicalPath = "/private/caller/repo/a"
			},
		},
		{
			name:   "name mismatch",
			change: func(candidate *Candidate) { candidate.Name = "private_name" },
		},
		{
			name: "module ID mismatch",
			change: func(candidate *Candidate) {
				candidate.ModuleID = "private-module-id"
			},
		},
		{
			name: "module path mismatch",
			change: func(candidate *Candidate) {
				candidate.ModulePath = "example.com/private/module"
			},
		},
		{
			name: "package directory mismatch",
			change: func(candidate *Candidate) {
				candidate.PackageDir = "private/directory"
			},
		},
		{
			name: "module-relative directory mismatch",
			change: func(candidate *Candidate) {
				candidate.ModuleRelativeDir = "private/relative"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.change(&candidate)
			selection, err := New(Input{
				Graph: graph,
				Candidates: []Candidate{
					valid,
					candidate,
				},
			})
			if !errors.Is(err, errUnauthorized) {
				t.Fatalf("New error = %v, want unauthorized", err)
			}
			if got := selection.Packages(); got != nil {
				t.Fatalf("failed selection exposed prefix: %#v", got)
			}
			for _, scalar := range [...]string{
				candidate.CanonicalPath,
				candidate.Name,
				candidate.ModuleID,
				candidate.ModulePath,
				candidate.PackageDir,
				candidate.ModuleRelativeDir,
			} {
				if len(scalar) >= 8 && strings.Contains(err.Error(), scalar) {
					t.Fatalf("error echoed caller scalar %q: %v", scalar, err)
				}
			}
			if strings.Contains(err.Error(), "/private/") {
				t.Fatalf("error exposed an absolute root: %v", err)
			}
		})
	}

	conflicting := valid
	conflicting.Name = "conflicting-private-name"
	selection, err := New(Input{
		Graph:      graph,
		Candidates: []Candidate{valid, conflicting},
	})
	if !errors.Is(err, errUnauthorized) || selection.Packages() != nil {
		t.Fatalf("conflicting duplicate result = %#v, %v", selection, err)
	}
}

func TestSelectionRequiresGraphAuthorization(t *testing.T) {
	valid := candidateFromFact(defaultSelectionFacts().Packages[0])
	emptyGraph := newPackageSelectionTestGraph(
		t,
		"/definitely-not-present/package-empty",
		"/definitely-not-present/package-empty",
		gofacts.Facts{},
	)
	for name, graph := range map[string]workspacegraph.Graph{
		"empty graph": emptyGraph,
		"zero graph":  {},
	} {
		t.Run(name, func(t *testing.T) {
			selection, err := New(Input{
				Graph:      graph,
				Candidates: []Candidate{valid},
			})
			if !errors.Is(err, errUnauthorized) || selection.Packages() != nil {
				t.Fatalf("New result = %#v, %v", selection, err)
			}
		})
	}
}

func TestSelectionExcludesFilesEditorialFieldsAndUnselectedPackages(t *testing.T) {
	facts := defaultSelectionFacts()
	facts.Packages[0].Files = []string{"a/a.go"}
	graph := newPackageSelectionTestGraph(
		t,
		"/definitely-not-present/package-fields",
		"/definitely-not-present/package-fields",
		facts,
	)
	candidate := candidateFromFact(facts.Packages[0])
	selection, err := New(Input{
		Graph:      graph,
		Candidates: []Candidate{candidate},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := selection.Packages(); !reflect.DeepEqual(
		got,
		[]Package{packageFromCandidate(candidate)},
	) {
		t.Fatalf("Packages = %#v", got)
	}

	packageType := reflect.TypeOf(Package{})
	var fields []string
	for index := 0; index < packageType.NumField(); index++ {
		fields = append(fields, packageType.Field(index).Name)
	}
	wantFields := []string{
		"CanonicalPath",
		"Name",
		"ModuleID",
		"ModulePath",
		"PackageDir",
		"ModuleRelativeDir",
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("Package fields = %#v, want %#v", fields, wantFields)
	}
}

func TestSelectionPreflightBudgetPrecedence(t *testing.T) {
	oversized := strings.Repeat("x", MaxScalarBytes+1)

	t.Run("raw count before scalar", func(t *testing.T) {
		candidates := make([]Candidate, MaxRows+1)
		candidates[0].CanonicalPath = oversized
		_, err := New(Input{Candidates: candidates})
		if !errors.Is(err, errRawBounds) {
			t.Fatalf("New error = %v, want raw bounds", err)
		}
	})

	scalarFields := []struct {
		name string
		set  func(*Candidate)
	}{
		{name: "canonical path", set: func(c *Candidate) { c.CanonicalPath = oversized }},
		{name: "name", set: func(c *Candidate) { c.Name = oversized }},
		{name: "module ID", set: func(c *Candidate) { c.ModuleID = oversized }},
		{name: "module path", set: func(c *Candidate) { c.ModulePath = oversized }},
		{name: "package directory", set: func(c *Candidate) { c.PackageDir = oversized }},
		{
			name: "module-relative directory",
			set:  func(c *Candidate) { c.ModuleRelativeDir = oversized },
		},
	}
	for _, field := range scalarFields {
		t.Run("individual "+field.name, func(t *testing.T) {
			candidate := Candidate{}
			field.set(&candidate)
			_, err := New(Input{Candidates: []Candidate{candidate}})
			if !errors.Is(err, errScalarBounds) {
				t.Fatalf("New error = %v, want scalar bounds", err)
			}
		})
	}

	t.Run("all individual scalars before aggregate", func(t *testing.T) {
		candidates := exactAggregateCandidates()
		candidates = append(candidates, Candidate{CanonicalPath: oversized})
		_, err := New(Input{Candidates: candidates})
		if !errors.Is(err, errScalarBounds) {
			t.Fatalf("New error = %v, want scalar bounds", err)
		}
	})

	t.Run("aggregate scalar budget", func(t *testing.T) {
		candidates := exactAggregateCandidates()
		candidates[len(candidates)-1].ModuleRelativeDir =
			strings.Repeat("z", MaxScalarBytes)
		_, err := New(Input{Candidates: candidates})
		if !errors.Is(err, errAggregateBounds) {
			t.Fatalf("New error = %v, want aggregate bounds", err)
		}
	})

	t.Run("exact aggregate bound reaches authority", func(t *testing.T) {
		_, err := New(Input{Candidates: exactAggregateCandidates()})
		if !errors.Is(err, errUnauthorized) {
			t.Fatalf("New error = %v, want authority lookup", err)
		}
	})
}

func TestOversizedScalarPreflightDoesNotAllocate(t *testing.T) {
	input := Input{Candidates: []Candidate{{
		Name: strings.Repeat("x", MaxScalarBytes+1),
	}}}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, benchmarkPackageSelectionError = New(input)
	}); allocations != 0 {
		t.Fatalf("oversized preflight allocations = %v, want 0", allocations)
	}
	if !errors.Is(benchmarkPackageSelectionError, errScalarBounds) {
		t.Fatalf("New error = %v, want scalar bounds", benchmarkPackageSelectionError)
	}
}

func BenchmarkNewPreflightOversizedScalar(b *testing.B) {
	input := Input{Candidates: []Candidate{{
		Name: strings.Repeat("x", MaxScalarBytes+1),
	}}}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, benchmarkPackageSelectionError = New(input)
	}
}

func TestWorkspacePackageSelectionProductionDependenciesStayNeutral(t *testing.T) {
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
				t.Fatalf(
					"production dependency %q is outside the neutral contract",
					importPath,
				)
			}
		}
	}
}

func TestSelectionErrorsDoNotExposeAbsoluteRoots(t *testing.T) {
	root := "/definitely-not-present/private-workspace-package"
	_, err := New(Input{Candidates: []Candidate{{CanonicalPath: root}}})
	if err == nil {
		t.Fatal("New unexpectedly succeeded")
	}
	if strings.Contains(fmt.Sprint(err), root) {
		t.Fatalf("error exposed absolute root: %v", err)
	}
}

func defaultSelectionFacts() gofacts.Facts {
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{
			{
				ID: "root-id", ModulePath: packageRoot, ModuleDir: ".", Main: true,
			},
			{
				ID: "tools-id", ModulePath: "example.com/repo/tools",
				ModuleDir: "tools",
			},
		},
		Packages: []gofacts.PackageFact{
			packageFact(packageA, "a", "root-id", packageRoot, "a", "a"),
			packageFact(packageB, "b", "root-id", packageRoot, "b", "b"),
			packageFact(packageC, "c", "root-id", packageRoot, "c", "c"),
			packageFact(
				packageTool,
				"main",
				"tools-id",
				"example.com/repo/tools",
				"tools/cmd/tool",
				"cmd/tool",
			),
			packageFact(packageRoot, "repo", "root-id", packageRoot, ".", "."),
		},
	}
}

func packageFact(
	canonicalPath string,
	name string,
	moduleID string,
	modulePath string,
	packageDir string,
	moduleRelativeDir string,
) gofacts.PackageFact {
	return gofacts.PackageFact{
		CanonicalPath:     canonicalPath,
		Name:              name,
		ModuleID:          moduleID,
		ModulePath:        modulePath,
		PackageDir:        packageDir,
		ModuleRelativeDir: moduleRelativeDir,
	}
}

func candidateFromFact(fact gofacts.PackageFact) Candidate {
	return Candidate{
		CanonicalPath:     fact.CanonicalPath,
		Name:              fact.Name,
		ModuleID:          fact.ModuleID,
		ModulePath:        fact.ModulePath,
		PackageDir:        fact.PackageDir,
		ModuleRelativeDir: fact.ModuleRelativeDir,
	}
}

func packageFromCandidate(candidate Candidate) Package {
	return Package{
		CanonicalPath:     candidate.CanonicalPath,
		Name:              candidate.Name,
		ModuleID:          candidate.ModuleID,
		ModulePath:        candidate.ModulePath,
		PackageDir:        candidate.PackageDir,
		ModuleRelativeDir: candidate.ModuleRelativeDir,
	}
}

func newPackageSelectionTestGraph(
	t *testing.T,
	repositoryRoot string,
	analysisRoot string,
	facts gofacts.Facts,
) workspacegraph.Graph {
	t.Helper()
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: analysisRoot,
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
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  facts,
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	return graph
}

func exactAggregateCandidates() []Candidate {
	scalar := strings.Repeat("x", MaxScalarBytes)
	fullRows := MaxAggregateScalarBytes / (6 * MaxScalarBytes)
	candidates := make([]Candidate, fullRows+1)
	for index := range candidates[:fullRows] {
		candidates[index] = Candidate{
			CanonicalPath:     scalar,
			Name:              scalar,
			ModuleID:          scalar,
			ModulePath:        scalar,
			PackageDir:        scalar,
			ModuleRelativeDir: scalar,
		}
	}
	remainingFields := (MaxAggregateScalarBytes -
		fullRows*6*MaxScalarBytes) / MaxScalarBytes
	last := &candidates[len(candidates)-1]
	fields := []*string{
		&last.CanonicalPath,
		&last.Name,
		&last.ModuleID,
		&last.ModulePath,
		&last.PackageDir,
		&last.ModuleRelativeDir,
	}
	for index := 0; index < remainingFields; index++ {
		*fields[index] = scalar
	}
	return candidates
}
