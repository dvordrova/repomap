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
	facts.PackageOrigins = []gofacts.PackageOrigin{
		{PackagePath: "github.com/dvordrova/repomap/cmd/repomap"},
		{PackagePath: "net/http", Standard: true},
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
	if !slices.Equal(scoped.PackageOrigins, facts.PackageOrigins) {
		t.Fatalf("scoped package origins = %#v, want complete %#v", scoped.PackageOrigins, facts.PackageOrigins)
	}
	scoped.PackageOrigins[0].PackagePath = "mutated"
	if facts.PackageOrigins[0].PackagePath == "mutated" {
		t.Fatal("ScopeGoFacts aliased complete package-origin authority")
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
		// A raw non-DepOnly row can own dependency authority without becoming
		// a build-selected PackageFact. It must not block an unrelated target.
		{Language: "go", Name: "ghost", ModulePath: modulePath, PackagePath: modulePath + "/ghost", RepositoryPath: "ghost"},
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
		{
			Language: "go", Kind: dependencies.KindWorkspace, Name: "report", ModulePath: modulePath,
			PackagePath: modulePath + "/internal/report", RepositoryPath: "internal/report",
			ImporterRefs: []string{refs[modulePath+"/cmd/tool"]},
		},
		{Language: "go", Kind: dependencies.KindStdlib, Name: "html", PackagePath: "html", ImporterRefs: []string{refs[modulePath+"/internal/report"]}},
		{Language: "go", Kind: dependencies.KindStdlib, Name: "os", PackagePath: "os", ImporterRefs: []string{refs[modulePath+"/cmd/other"]}},
		{Language: "go", Kind: dependencies.KindStdlib, Name: "bytes", PackagePath: "bytes", ImporterRefs: []string{refs[modulePath+"/ghost"]}},
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
	if scoped.Dependencies == nil || len(scoped.Dependencies.Importers) != 2 || len(scoped.Dependencies.Dependencies) != 3 {
		t.Fatalf("scoped dependencies = %#v", scoped.Dependencies)
	}
	if !slices.Equal(scoped.InternalEdges, []gofacts.Edge{{
		From: modulePath + "/cmd/tool", To: modulePath + "/internal/report",
	}}) {
		t.Fatalf("scoped internal edges = %#v", scoped.InternalEdges)
	}
	for _, value := range scoped.Dependencies.Dependencies {
		if value.PackagePath == "os" {
			t.Fatalf("dependency from excluded package survived: %#v", scoped.Dependencies.Dependencies)
		}
	}
	if len(facts.Dependencies.Importers) != 4 || len(facts.Dependencies.Dependencies) != 5 {
		t.Fatal("ScopeGoFacts mutated the source dependency catalog")
	}
}

func TestScopeGoFactsSeparatesEqualPackagePathsAcrossRepositoryContexts(t *testing.T) {
	t.Parallel()

	const sharedPackage = "example.com/shared"
	facts := gofacts.Facts{}
	appendCatalogModule(&facts, gofacts.ModuleFact{
		ID: "module-app-one", ModulePath: "example.com/app-one", ModuleDir: "apps/one", Main: true,
	}, []syntheticPackage{{path: "example.com/app-one", dir: ".", executable: true, line: 3}})
	appendCatalogModule(&facts, gofacts.ModuleFact{
		ID: "module-app-two", ModulePath: "example.com/app-two", ModuleDir: "apps/two", Main: true,
	}, []syntheticPackage{{path: "example.com/app-two", dir: ".", executable: true, line: 3}})
	appendCatalogModule(&facts, gofacts.ModuleFact{
		ID: "module-shared-one", ModulePath: sharedPackage, ModuleDir: "shared/one", Main: true,
	}, []syntheticPackage{{path: sharedPackage, dir: "."}})
	appendCatalogModule(&facts, gofacts.ModuleFact{
		ID: "module-shared-two", ModulePath: sharedPackage, ModuleDir: "shared/two", Main: true,
	}, []syntheticPackage{{path: sharedPackage, dir: "."}})

	importers := []dependencies.Importer{
		{
			Language: "go", Name: "main", ModulePath: "example.com/app-one",
			PackagePath: "example.com/app-one", RepositoryPath: "apps/one",
		},
		{
			Language: "go", Name: "main", ModulePath: "example.com/app-two",
			PackagePath: "example.com/app-two", RepositoryPath: "apps/two",
		},
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
		{
			Language: "go", Kind: dependencies.KindWorkspace, Name: "shared", ModulePath: sharedPackage,
			PackagePath: sharedPackage, RepositoryPath: "shared/one",
			ImporterRefs: []string{refs["example.com/app-one"]},
		},
		{
			Language: "go", Kind: dependencies.KindWorkspace, Name: "shared", ModulePath: sharedPackage,
			PackagePath: sharedPackage, RepositoryPath: "shared/two",
			ImporterRefs: []string{refs["example.com/app-two"]},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	facts.Dependencies = &catalog
	facts.InternalEdges = []gofacts.Edge{
		{From: "example.com/app-one", To: sharedPackage},
		{From: "example.com/app-two", To: sharedPackage},
	}

	for _, test := range []struct {
		name           string
		packageDir     string
		packagePath    string
		sharedDir      string
		otherSharedDir string
	}{
		{
			name: "first context", packageDir: "apps/one", packagePath: "example.com/app-one",
			sharedDir: "shared/one", otherSharedDir: "shared/two",
		},
		{
			name: "second context", packageDir: "apps/two", packagePath: "example.com/app-two",
			sharedDir: "shared/two", otherSharedDir: "shared/one",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := requireCandidateTarget(t, facts, KindExecutablePackage, test.packageDir)
			scoped, scopeErr := ScopeGoFacts(facts, target)
			if scopeErr != nil {
				t.Fatal(scopeErr)
			}
			gotPackages := make(map[string]bool, len(scoped.Packages))
			for _, pkg := range scoped.Packages {
				gotPackages[pkg.PackageDir] = true
			}
			if len(scoped.Packages) != 2 || !gotPackages[test.packageDir] || !gotPackages[test.sharedDir] ||
				gotPackages[test.otherSharedDir] {
				t.Fatalf("exact scoped packages = %#v", scoped.Packages)
			}
			if !slices.Equal(scoped.InternalEdges, []gofacts.Edge{{
				From: test.packagePath, To: sharedPackage,
			}}) {
				t.Fatalf("exact scoped internal edges = %#v", scoped.InternalEdges)
			}
			if scoped.Dependencies == nil || len(scoped.Dependencies.Dependencies) != 1 ||
				scoped.Dependencies.Dependencies[0].RepositoryPath != test.sharedDir {
				t.Fatalf("exact scoped dependencies = %#v", scoped.Dependencies)
			}
		})
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
