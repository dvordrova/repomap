package orient

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestRunDeliversSurfaceResult(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "go.mod", "module example.com/orient-surface\n\ngo 1.24\n")
	writeSurfaceTestFile(t, repository, "main.go", `package main

import "net/http"

func health(http.ResponseWriter, *http.Request) {}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	_ = http.ListenAndServe(":8080", mux)
}
`)
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "go.mod", "main.go")

	debugDirectory := t.TempDir()
	var directCallIndex *surfacediscovery.DirectCallIndex
	err := Run(context.Background(), prepareOrientRunOptions(t, repository, Options{
		RepoPath: repository,
		DebugDir: debugDirectory, RunID: "surface-run", RequireArtifacts: true,
		AnalyzeGoProgram: true, AutoGoTarget: true,
		DirectCallIndexSink: func(index surfacediscovery.DirectCallIndex) {
			directCallIndex = &index
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if directCallIndex == nil || directCallIndex.State != surfacediscovery.DirectCallIndexReady ||
		len(directCallIndex.Nodes) == 0 || len(directCallIndex.Edges) != 0 ||
		directCallIndex.Coverage.NonRepositoryCallsExcluded == 0 || directCallIndex.SHA256 == "" {
		t.Fatalf("in-memory direct-call index = %#v", directCallIndex)
	}
}

func TestSurfaceDiscoveryInputProjectsModuleLibraryRootsAndFullOwningModuleScope(t *testing.T) {
	const modulePath = "example.com/library"
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "root", ModulePath: modulePath, ModuleDir: ".", Main: true,
			PackagesCount: 3, RetainedPackagesCount: 3,
			Coverage: gofacts.ModuleCoverage{PackagesDiscovered: 3, PackagesRetained: 3},
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: modulePath, Name: "library", ModuleID: "root", ModulePath: modulePath,
				PackageDir: ".", ModuleRelativeDir: ".", Locality: "local", DeclarationsScanned: true,
				LoadCompleteness: completeSurfacePackageLoad(),
				Declarations:     []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "Open"}},
			},
			{
				CanonicalPath: modulePath + "/client", Name: "client", ModuleID: "root", ModulePath: modulePath,
				PackageDir: "client", ModuleRelativeDir: "client", Locality: "local", DeclarationsScanned: true,
				LoadCompleteness: completeSurfacePackageLoad(),
				Declarations:     []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationType, Name: "Client"}},
			},
			{
				CanonicalPath: modulePath + "/internal/state", Name: "state", ModuleID: "root", ModulePath: modulePath,
				PackageDir: "internal/state", ModuleRelativeDir: "internal/state", Locality: "local", DeclarationsScanned: true,
				LoadCompleteness: completeSurfacePackageLoad(),
			},
		},
	}
	catalog, err := analysistarget.BuildCatalog(facts)
	if err != nil || len(catalog.Entries) != 1 {
		t.Fatalf("module-library catalog = %#v / %v", catalog, err)
	}
	target := catalog.Entries[0].Candidate.Target
	scoped, err := analysistarget.ScopeGoFacts(facts, target)
	if err != nil {
		t.Fatal(err)
	}
	input := surfaceDiscoveryInput("library", &scoped, &target)
	if input.AnalysisTarget == nil || input.AnalysisTarget.TargetRef != target.Ref ||
		input.AnalysisTarget.Kind != "module_library" || input.AnalysisTarget.ModuleID != "root" ||
		input.AnalysisTarget.ModulePath != modulePath || input.AnalysisTarget.ModuleDir != "." ||
		input.AnalysisTarget.PackagePath != "" ||
		!reflect.DeepEqual(input.AnalysisTarget.TargetPackages, []string{modulePath, modulePath + "/client"}) ||
		len(input.AnalysisTarget.Roots) != 0 {
		t.Fatalf("module-library target adapter = %#v", input.AnalysisTarget)
	}
	gotPackages := make([]string, 0, len(input.Packages))
	for _, pkg := range input.Packages {
		gotPackages = append(gotPackages, pkg.Path)
	}
	if !reflect.DeepEqual(gotPackages, []string{modulePath, modulePath + "/client", modulePath + "/internal/state"}) {
		t.Fatalf("module-library admitted package scope = %#v", input.Packages)
	}
}

func completeSurfacePackageLoad() *gofacts.PackageLoadCompleteness {
	return &gofacts.PackageLoadCompleteness{
		Version: gofacts.PackageLoadCompletenessVersion,
		State:   gofacts.PackageLoadComplete,
	}
}

func TestRunSurfaceDiscoveryWithoutArtifactDirectoryFails(t *testing.T) {
	repository := t.TempDir()
	writeSurfaceTestFile(t, repository, "README.md", "fixture\n")
	runOrientGit(t, repository, "init", "--quiet")
	runOrientGit(t, repository, "add", "--", "README.md")

	err := Run(context.Background(), prepareOrientRunOptions(t, repository, Options{
		RepoPath: repository, AnalyzeGoProgram: true,
	}))
	if err == nil {
		t.Fatal("surface discovery without an artifact directory was silently skipped")
	}
}

func writeSurfaceTestFile(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func prepareOrientRunOptions(t *testing.T, repository string, options Options) Options {
	t.Helper()
	repositoryCorpus, err := corpus.Open(t.Context(), repository)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repositoryCorpus.Close() })
	options.RepoPath = repository
	options.RepositoryCorpus = repositoryCorpus
	if options.GoTarget == "" {
		options.GoTarget = runtime.GOOS + "/" + runtime.GOARCH
	}
	if options.AnalysisTargetSelector == nil {
		options.AnalysisTargetSelector = func(
			_ context.Context,
			_ string,
			catalog analysistarget.TargetCatalog,
			_ gofacts.Facts,
		) (snapshot.TargetRunSelection, error) {
			if len(catalog.Entries) == 0 {
				return snapshot.TargetRunSelection{}, nil
			}
			ref := catalog.Entries[0].Candidate.Target.Ref
			return snapshot.TargetRunSelection{DefaultTargetRef: ref, TargetRefs: []string{ref}}, nil
		}
	}
	return options
}
