package analysistarget

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestScopeGoFactsRepomapExecutableDropsOtherCommands(t *testing.T) {
	facts := syntheticFacts("module-root", "github.com/dvordrova/repomap", []syntheticPackage{
		{path: "github.com/dvordrova/repomap/cmd/quality-evaluate", dir: "cmd/quality-evaluate", executable: true, line: 16},
		{path: "github.com/dvordrova/repomap/cmd/repomap", dir: "cmd/repomap", executable: true, line: 43},
		{path: "github.com/dvordrova/repomap/internal/report", dir: "internal/report"},
	})
	facts.InternalEdges = []gofacts.Edge{
		{From: "github.com/dvordrova/repomap/cmd/repomap", To: "github.com/dvordrova/repomap/internal/report"},
	}
	target := requireCandidateTarget(t, facts, KindExecutablePackage, "cmd/repomap")
	scoped, err := ScopeGoFacts(facts, target)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, scoped, []string{
		"github.com/dvordrova/repomap/cmd/repomap",
		"github.com/dvordrova/repomap/internal/report",
	})
	if len(scoped.EntrypointPackages) != 1 || scoped.EntrypointPackages[0].ImportPath != target.PackagePath {
		t.Fatalf("entrypoints = %#v", scoped.EntrypointPackages)
	}
	if len(facts.Packages) != 3 {
		t.Fatal("ScopeGoFacts mutated its input")
	}
}

func TestScopeGoFactsFiltersDependencyImportersWithTheExactPackageScope(t *testing.T) {
	t.Parallel()

	const modulePath = "example.com/tool"
	facts := syntheticFacts("module-root", modulePath, []syntheticPackage{
		{path: modulePath + "/cmd/other", dir: "cmd/other", executable: true, line: 5},
		{path: modulePath + "/cmd/tool", dir: "cmd/tool", executable: true, line: 7},
		{path: modulePath + "/internal/report", dir: "internal/report"},
	})
	facts.InternalEdges = []gofacts.Edge{{From: modulePath + "/cmd/tool", To: modulePath + "/internal/report"}}
	importers := []dependencies.Importer{
		{Language: "go", Name: "other", ModulePath: modulePath, PackagePath: modulePath + "/cmd/other", RepositoryPath: "cmd/other"},
		{Language: "go", Name: "tool", ModulePath: modulePath, PackagePath: modulePath + "/cmd/tool", RepositoryPath: "cmd/tool"},
		{Language: "go", Name: "report", ModulePath: modulePath, PackagePath: modulePath + "/internal/report", RepositoryPath: "internal/report"},
	}
	sealedImporters, err := dependencies.BuildWithOmissions(importers, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	refs := make(map[string]string, len(sealedImporters.Importers))
	for _, importer := range sealedImporters.Importers {
		refs[importer.PackagePath] = importer.Ref
	}
	catalog, err := dependencies.BuildWithOmissions(sealedImporters.Importers, []dependencies.Dependency{
		{Language: "go", Kind: dependencies.KindStdlib, Name: "fmt", PackagePath: "fmt", ImporterRefs: []string{refs[modulePath+"/cmd/tool"]}},
		{Language: "go", Kind: dependencies.KindStdlib, Name: "html", PackagePath: "html", ImporterRefs: []string{refs[modulePath+"/internal/report"]}},
		{Language: "go", Kind: dependencies.KindStdlib, Name: "os", PackagePath: "os", ImporterRefs: []string{refs[modulePath+"/cmd/other"]}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	facts.Dependencies = &catalog

	target := requireCandidateTarget(t, facts, KindExecutablePackage, "cmd/tool")
	scoped, err := ScopeGoFacts(facts, target)
	if err != nil {
		t.Fatal(err)
	}
	if scoped.Dependencies == nil || len(scoped.Dependencies.Importers) != 2 || len(scoped.Dependencies.Dependencies) != 2 {
		t.Fatalf("scoped dependencies = %#v", scoped.Dependencies)
	}
	for _, value := range scoped.Dependencies.Dependencies {
		if value.PackagePath == "os" {
			t.Fatalf("dependency from excluded package survived: %#v", scoped.Dependencies.Dependencies)
		}
	}
	if len(facts.Dependencies.Importers) != 3 || len(facts.Dependencies.Dependencies) != 3 {
		t.Fatal("ScopeGoFacts mutated the source dependency catalog")
	}
}

func TestScopeGoFactsMobyOverrideAndTelebotRootLibrary(t *testing.T) {
	moby := syntheticFacts("module-root", "github.com/moby/moby/v2", []syntheticPackage{
		{path: "github.com/moby/moby/v2/cmd/docker-proxy", dir: "cmd/docker-proxy", executable: true, line: 20},
		{path: "github.com/moby/moby/v2/cmd/dockerd", dir: "cmd/dockerd", executable: true, line: 16},
		{path: "github.com/moby/moby/v2/daemon", dir: "daemon"},
	})
	moby.InternalEdges = []gofacts.Edge{{From: "github.com/moby/moby/v2/cmd/dockerd", To: "github.com/moby/moby/v2/daemon"}}
	selected := requireCandidateTarget(t, moby, KindExecutablePackage, "cmd/dockerd")
	scoped, err := ScopeGoFacts(moby, selected)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, scoped, []string{"github.com/moby/moby/v2/cmd/dockerd", "github.com/moby/moby/v2/daemon"})

	telebot := syntheticFacts("module-root", "gopkg.in/telebot.v3", []syntheticPackage{
		{path: "gopkg.in/telebot.v3", dir: "."},
		{path: "gopkg.in/telebot.v3/layout", dir: "layout"},
		{path: "gopkg.in/telebot.v3/middleware", dir: "middleware"},
	})
	library := requireCandidateTarget(t, telebot, KindModuleLibrary, "")
	telebotScoped, err := ScopeGoFacts(telebot, library)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, telebotScoped, []string{"gopkg.in/telebot.v3", "gopkg.in/telebot.v3/layout", "gopkg.in/telebot.v3/middleware"})
	if len(telebotScoped.EntrypointPackages) != 0 {
		t.Fatalf("library entrypoints = %#v", telebotScoped.EntrypointPackages)
	}
}

func TestTargetValidateRejectsRefDrift(t *testing.T) {
	facts := syntheticFacts("module-root", "gopkg.in/telebot.v3", []syntheticPackage{{path: "gopkg.in/telebot.v3", dir: "."}})
	target := requireCandidateTarget(t, facts, KindModuleLibrary, "")
	if _, err := target.CanonicalJSON(); err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	target.PackagePath += "/drift"
	if err := target.Validate(); err == nil {
		t.Fatal("Validate accepted a drifted self-seal")
	}
}

func TestScopeGoFactsModuleLibraryUsesExactNonMainModuleInventory(t *testing.T) {
	facts := syntheticFacts("module-root", "example.com/mixed", []syntheticPackage{
		{path: "example.com/mixed", dir: ".", executable: true, line: 4},
		{path: "example.com/mixed/api", dir: "api"},
		{path: "example.com/mixed/internal/store", dir: "internal/store"},
	})
	candidates, err := Candidates(facts)
	if err != nil {
		t.Fatal(err)
	}
	var target Target
	for _, candidate := range candidates {
		if candidate.Target.Kind == KindModuleLibrary {
			target = candidate.Target
		}
	}
	if target.Ref == "" {
		t.Fatalf("no module-library candidate: %#v", candidates)
	}

	scoped, err := ScopeGoFacts(facts, target)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, scoped, []string{
		"example.com/mixed/api",
		"example.com/mixed/internal/store",
	})
	if len(scoped.EntrypointPackages) != 0 || len(scoped.Modules) != 1 ||
		scoped.Modules[0].PackagesCount != 2 {
		t.Fatalf("module-library scope = %#v", scoped)
	}

	driftedFacts := facts
	driftedFacts.Packages = append([]gofacts.PackageFact(nil), facts.Packages[:2]...)
	if _, err := ScopeGoFacts(driftedFacts, target); err == nil ||
		!strings.Contains(err.Error(), "module package inventory mismatch") {
		t.Fatalf("scope accepted drifted module inventory: %v", err)
	}
}

func assertPackagePaths(t *testing.T, facts gofacts.Facts, want []string) {
	t.Helper()
	got := make([]string, 0, len(facts.Packages))
	for _, pkg := range facts.Packages {
		got = append(got, pkg.CanonicalPath)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
}
