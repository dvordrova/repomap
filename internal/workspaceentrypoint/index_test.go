package workspaceentrypoint

import (
	"crypto/sha256"
	"errors"
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
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

func TestIndexRetainsExactRootAndNestedModuleEntrypoints(t *testing.T) {
	const repositoryRoot = "/definitely-not-present/workspace-entrypoint-root"
	modules := []gofacts.ModuleFact{
		{ID: "tools-id", ModulePath: "example.com/repo/tools", ModuleDir: "tools"},
		{ID: "root-id", ModulePath: "example.com/repo", ModuleDir: "."},
	}
	packages := []gofacts.PackageFact{
		{
			CanonicalPath: "example.com/repo/tools/cmd/tool", Name: "main",
			ModuleID: "tools-id", ModulePath: "example.com/repo/tools",
			PackageDir: "tools/cmd/tool", ModuleRelativeDir: "cmd/tool",
			Files: []string{"tools/cmd/tool/main.go"},
		},
		{
			CanonicalPath: "example.com/repo/cmd/app", Name: "main",
			ModuleID: "root-id", ModulePath: "example.com/repo",
			PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
			Files: []string{"cmd/app/generated.go", "cmd/app/main.go"},
		},
	}
	graph := newTestGraph(
		t,
		repositoryRoot,
		repositoryRoot,
		[]string{"cmd/app/main.go", "tools/cmd/tool/main.go"},
		modules,
		packages,
	)
	facts := gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{
		testEntrypoint(
			"example.com/repo/tools",
			"example.com/repo/tools/cmd/tool",
			"tools/cmd/tool",
			"cmd/tool",
			"tools",
			"tools/cmd/tool/main.go",
			7,
		),
		testEntrypoint(
			"example.com/repo",
			"example.com/repo/cmd/app",
			"cmd/app",
			"cmd/app",
			".",
			"cmd/app/main.go",
			9,
		),
	}}
	duplicate := facts.EntrypointPackages[1]
	duplicate.Kind = "different-editorial-role"
	duplicate.Dir = "ignored-editorial-directory"
	duplicate.GoFiles = []string{"ignored.go"}
	facts.EntrypointPackages = append(facts.EntrypointPackages, duplicate)

	index, err := New(Input{GoFacts: facts, Graph: graph})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := []Entry{
		{
			Kind: string(gofacts.EntrypointAnchorGoMain), Package: "example.com/repo/cmd/app",
			Path: "cmd/app/main.go", Symbol: "main", Line: 9, Openable: true,
		},
		{
			Kind: string(gofacts.EntrypointAnchorGoMain), Package: "example.com/repo/tools/cmd/tool",
			Path: "tools/cmd/tool/main.go", Symbol: "main", Line: 7, Openable: true,
		},
	}
	if got := index.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Entries = %#v, want %#v", got, want)
	} else if cap(got) > MaxRawRows {
		t.Fatalf("Entries capacity = %d, want <= %d", cap(got), MaxRawRows)
	}
	if got, ok := index.Lookup("example.com/repo/cmd/app", "cmd/app/main.go", 9); !ok || got != want[0] {
		t.Fatalf("Lookup = %#v, %t, want %#v, true", got, ok, want[0])
	}

	// Both caller-owned facts and returned slices remain non-authoritative.
	facts.EntrypointPackages[1].ImportPath = "mutated.example"
	first := index.Entries()
	first[0].Path = "mutated.go"
	if got := index.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("mutations changed immutable index: %#v", got)
	}
	if _, ok := index.Lookup(strings.Repeat("x", MaxScalarBytes+1), "cmd/app/main.go", 9); ok {
		t.Fatal("Lookup accepted an oversized package")
	}
	if _, ok := index.Lookup("example.com/repo/cmd/app", repositoryRoot+"/cmd/app/main.go", 9); ok {
		t.Fatal("Lookup accepted an absolute path")
	}
	if exposed := fmt.Sprintf("%#v", index.Entries()); strings.Contains(exposed, repositoryRoot) {
		t.Fatalf("index exposed absolute root: %s", exposed)
	}
}

func TestIndexUsesAnalysisRelativePathsForSubdirectoryRoot(t *testing.T) {
	const repositoryRoot = "/definitely-not-present/workspace-entrypoint-subdirectory"
	const analysisRoot = repositoryRoot + "/service"
	modules := []gofacts.ModuleFact{{
		ID: "service-id", ModulePath: "example.com/service", ModuleDir: ".",
	}}
	packages := []gofacts.PackageFact{{
		CanonicalPath: "example.com/service/cmd/app", Name: "main",
		ModuleID: "service-id", ModulePath: "example.com/service",
		PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
		Files: []string{"cmd/app/main.go"},
	}}
	graph := newTestGraph(
		t,
		repositoryRoot,
		analysisRoot,
		[]string{"cmd/app/main.go"},
		modules,
		packages,
	)
	facts := gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{
		testEntrypoint(
			"example.com/service",
			"example.com/service/cmd/app",
			"cmd/app",
			"cmd/app",
			".",
			"cmd/app/main.go",
			3,
		),
	}}
	index, err := New(Input{GoFacts: facts, Graph: graph})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	entries := index.Entries()
	if len(entries) != 1 || entries[0].Path != "cmd/app/main.go" || !entries[0].Openable {
		t.Fatalf("subdirectory entries = %#v", entries)
	}
	if exposed := fmt.Sprintf("%#v", entries); strings.Contains(exposed, repositoryRoot) ||
		strings.Contains(exposed, "service/cmd/app/main.go") {
		t.Fatalf("subdirectory index leaked or re-prefixed root: %s", exposed)
	}
}

func TestIndexFiltersUntrustedRowsAfterPreflight(t *testing.T) {
	const repositoryRoot = "/definitely-not-present/workspace-entrypoint-filter"
	modules := []gofacts.ModuleFact{{
		ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
	}}
	packages := []gofacts.PackageFact{{
		CanonicalPath: "example.com/repo/cmd/app", Name: "main",
		ModuleID: "root-id", ModulePath: "example.com/repo",
		PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
		Files: []string{
			"cmd/app/generated.go",
			"cmd/app/invalid-kind.go",
			"cmd/app/invalid-version.go",
			"cmd/app/main.go",
		},
	}}
	graph := newTestGraph(
		t,
		repositoryRoot,
		repositoryRoot,
		[]string{"cmd/app/main.go"},
		modules,
		packages,
	)
	valid := testEntrypoint(
		"example.com/repo",
		"example.com/repo/cmd/app",
		"cmd/app",
		"cmd/app",
		".",
		"cmd/app/main.go",
		9,
	)
	valid.Anchors = append(valid.Anchors, gofacts.EntrypointAnchor{
		Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
		Path: "cmd/app/generated.go", Line: 4,
	}, gofacts.EntrypointAnchor{
		Version: gofacts.EntrypointAnchorVersion, Kind: "not-a-main-anchor",
		Path: "cmd/app/invalid-kind.go", Line: 5,
	}, gofacts.EntrypointAnchor{
		Version: 99, Kind: gofacts.EntrypointAnchorGoMain,
		Path: "cmd/app/invalid-version.go", Line: 6,
	}, gofacts.EntrypointAnchor{
		Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
		Path: "cmd/other/main.go", Line: 7,
	}, gofacts.EntrypointAnchor{
		Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
		Path: "../outside/main.go", Line: 8,
	})
	wrongOwner := testEntrypoint(
		"example.com/other",
		"example.com/repo/cmd/app",
		"cmd/app",
		"cmd/app",
		".",
		"cmd/app/main.go",
		10,
	)
	unknown := testEntrypoint(
		"example.com/repo",
		"example.com/repo/cmd/missing",
		"cmd/missing",
		"cmd/missing",
		".",
		"cmd/missing/main.go",
		11,
	)
	index, err := New(Input{
		GoFacts: gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{valid, wrongOwner, unknown}},
		Graph:   graph,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := []Entry{
		{
			Kind: string(gofacts.EntrypointAnchorGoMain), Package: "example.com/repo/cmd/app",
			Path: "cmd/app/generated.go", Symbol: "main", Line: 4, Openable: false,
		},
		{
			Kind: string(gofacts.EntrypointAnchorGoMain), Package: "example.com/repo/cmd/app",
			Path: "cmd/app/main.go", Symbol: "main", Line: 9, Openable: true,
		},
	}
	if got := index.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Entries = %#v, want %#v", got, want)
	}
}

func TestIndexRejectsConflictingExactIdentity(t *testing.T) {
	const repositoryRoot = "/definitely-not-present/workspace-entrypoint-conflict"
	modules := []gofacts.ModuleFact{{
		ID: "root-id", ModulePath: "example.com/repo", ModuleDir: ".",
	}}
	packages := []gofacts.PackageFact{{
		CanonicalPath: "example.com/repo/cmd/app", Name: "main",
		ModuleID: "root-id", ModulePath: "example.com/repo",
		PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
		Files: []string{"cmd/app/main.go"},
	}}
	graph := newTestGraph(
		t,
		repositoryRoot,
		repositoryRoot,
		[]string{"cmd/app/main.go"},
		modules,
		packages,
	)
	first := testEntrypoint(
		"example.com/repo",
		"example.com/repo/cmd/app",
		"cmd/app",
		"cmd/app",
		".",
		"cmd/app/main.go",
		9,
	)
	conflict := first
	conflict.ModuleDir = "different"
	_, err := New(Input{
		GoFacts: gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{first, conflict}},
		Graph:   graph,
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting exact identity") {
		t.Fatalf("New conflict error = %v", err)
	}
	if strings.Contains(err.Error(), repositoryRoot) ||
		strings.Contains(err.Error(), first.ImportPath) ||
		strings.Contains(err.Error(), first.Anchors[0].Path) {
		t.Fatalf("conflict error echoed caller data: %v", err)
	}
}

func TestIndexPreflightBudgetPrecedence(t *testing.T) {
	t.Run("outer raw count before scalar", func(t *testing.T) {
		rows := make([]gofacts.Entrypoint, MaxRawRows+1)
		rows[0].ModulePath = strings.Repeat("x", MaxScalarBytes+1)
		_, err := New(Input{GoFacts: gofacts.Facts{EntrypointPackages: rows}})
		if !errors.Is(err, errRawBounds) {
			t.Fatalf("New error = %v, want raw bounds", err)
		}
	})

	t.Run("aggregate anchor count before scalar", func(t *testing.T) {
		anchors := make([]gofacts.EntrypointAnchor, MaxRawRows+1)
		anchors[0].Path = strings.Repeat("x", MaxScalarBytes+1)
		_, err := New(Input{GoFacts: gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{{
			Anchors: anchors,
		}}}})
		if !errors.Is(err, errRawBounds) {
			t.Fatalf("New error = %v, want raw bounds", err)
		}
	})

	t.Run("individual scalar before graph construction", func(t *testing.T) {
		_, err := New(Input{GoFacts: gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: strings.Repeat("x", MaxScalarBytes+1),
		}}}})
		if !errors.Is(err, errScalarBounds) {
			t.Fatalf("New error = %v, want scalar bounds", err)
		}
	})

	t.Run("aggregate scalar budget", func(t *testing.T) {
		largeKind := gofacts.EntrypointAnchorKind(strings.Repeat("k", MaxScalarBytes))
		largePath := strings.Repeat("p", MaxScalarBytes)
		anchors := make([]gofacts.EntrypointAnchor, MaxRawRows)
		for index := range anchors {
			anchors[index] = gofacts.EntrypointAnchor{
				Version: 1, Kind: largeKind, Path: largePath, Line: index + 1,
			}
		}
		_, err := New(Input{GoFacts: gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{{
			Anchors: anchors,
		}}}})
		if !errors.Is(err, errAggregateBounds) {
			t.Fatalf("New error = %v, want aggregate bounds", err)
		}
	})

	t.Run("oversized scalar precedes aggregate remaining", func(t *testing.T) {
		budget := scalarBudget{remaining: 1}
		err := budget.consumeText(strings.Repeat("x", MaxScalarBytes+1))
		if !errors.Is(err, errScalarBounds) {
			t.Fatalf("consumeText error = %v, want scalar bounds", err)
		}
	})
}

func TestIndexRejectsUnavailableGraphWithoutLeakingFacts(t *testing.T) {
	_, err := New(Input{})
	if err == nil || err.Error() != "workspace entrypoint index: package graph is unavailable" {
		t.Fatalf("New error = %v", err)
	}
}

func TestWorkspaceEntrypointProductionDependenciesStayNeutral(t *testing.T) {
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
		"github.com/dvordrova/repomap/internal/gofacts":        true,
		"github.com/dvordrova/repomap/internal/workspacegraph": true,
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

func newTestGraph(
	t *testing.T,
	repositoryRoot,
	analysisRoot string,
	allowed []string,
	modules []gofacts.ModuleFact,
	packages []gofacts.PackageFact,
) workspacegraph.Graph {
	t.Helper()
	repositoryRoot = filepath.Clean(repositoryRoot)
	analysisRoot = filepath.Clean(analysisRoot)
	analysisRelative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	if analysisRelative == "." {
		analysisRelative = ""
	}
	captured := make([]freshness.CapturedInput, 0, len(allowed))
	for _, allowedPath := range allowed {
		repositoryPath := allowedPath
		if analysisRelative != "" {
			repositoryPath = path.Join(filepath.ToSlash(analysisRelative), allowedPath)
		}
		id := sha256.Sum256([]byte("id:" + repositoryPath))
		content := sha256.Sum256([]byte("content:" + repositoryPath))
		captured = append(captured, freshness.CapturedInput{
			Version: freshness.CapturedInputVersion,
			ID:      fmt.Sprintf("%x", id),
			Path:    repositoryPath,
			Kind:    freshness.FileRegular,
			Mode:    "100644",
			ContentSHA256: fmt.Sprintf(
				"%x",
				content,
			),
			Stages: []string{"workspace_entrypoint_test"},
		})
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot: analysisRoot,
		Repository: freshness.RepositoryState{
			Version: freshness.RepositoryStateVersion, Identity: repositoryRoot,
			Head: strings.Repeat("a", 40), Dirty: []freshness.DirtyFile{},
		},
		CapturedInputs: captured,
		AllowedPaths:   append([]string(nil), allowed...),
	})
	if err != nil {
		t.Fatalf("workspacesnapshot.New: %v", err)
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts: gofacts.Facts{
			Modules:  append([]gofacts.ModuleFact(nil), modules...),
			Packages: append([]gofacts.PackageFact(nil), packages...),
		},
	})
	if err != nil {
		t.Fatalf("workspacegraph.New: %v", err)
	}
	return graph
}

func testEntrypoint(
	modulePath,
	importPath,
	packageDir,
	moduleRelativeDir,
	moduleDir,
	anchorPath string,
	line int,
) gofacts.Entrypoint {
	return gofacts.Entrypoint{
		ModulePath: modulePath, ImportPath: importPath,
		Dir: packageDir, PackageDir: packageDir,
		ModuleRelativeDir: moduleRelativeDir, ModuleDir: moduleDir,
		Kind:    "editorial-role",
		GoFiles: []string{path.Base(anchorPath)},
		Anchors: []gofacts.EntrypointAnchor{{
			Version: gofacts.EntrypointAnchorVersion,
			Kind:    gofacts.EntrypointAnchorGoMain,
			Path:    anchorPath,
			Line:    line,
		}},
	}
}
