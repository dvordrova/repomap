package analysistarget

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestBuildCatalogUsesExecutablesAndOneModuleLibrarySurface(t *testing.T) {
	facts := syntheticFacts("module-root", "example.com/mixed", []syntheticPackage{
		{path: "example.com/mixed", dir: ".", executable: true, line: 5},
		{path: "example.com/mixed/api", dir: "api"},
		{path: "example.com/mixed/more", dir: "more"},
	})
	setPackageDeclarations(t, &facts, "example.com/mixed/api", true, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "Open", Path: "api/api.go", Line: 3, Column: 6, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationFunc, Name: "hidden", Path: "api/api.go", Line: 8, Column: 6, ExecutableBody: true},
	})
	setPackageDeclarations(t, &facts, "example.com/mixed/more", true, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationType, Name: "Client", Path: "more/client.go", Line: 4, Column: 6},
	})

	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Version != 4 || len(catalog.Entries) != 2 {
		t.Fatalf("catalog = %#v", catalog)
	}
	executable := requireCatalogPackage(t, catalog, "example.com/mixed")
	library := requireCatalogModuleLibrary(t, catalog, "module-root")
	if executable.Candidate.Target.Kind != KindExecutablePackage || executable.DisplayPath != "." {
		t.Fatalf("executable = %#v", executable)
	}
	if library.DisplayPath != "." || library.Candidate.Target.PackagePath != "" ||
		library.Candidate.Target.PackageDir != "" {
		t.Fatalf("module library invented package identity: %#v", library)
	}
	wantPackages := []TargetPackage{
		{PackagePath: "example.com/mixed/api", PackageDir: "api"},
		{PackagePath: "example.com/mixed/more", PackageDir: "more"},
	}
	if !slices.Equal(library.Candidate.Target.ModulePackages, wantPackages) ||
		!slices.Equal(library.Candidate.Target.LibraryPackages, wantPackages) ||
		len(library.PackageAPIs) != 2 || library.PackageAPIs[0].Package != wantPackages[0] {
		t.Fatalf("module library inventory = %#v", library)
	}
	if got := library.PackageAPIs[0].Declarations; len(got) != 1 || got[0].Name != "Open" ||
		got[0].Path != "" || got[0].Line != 0 || got[0].ExecutableBody {
		t.Fatalf("names-only package API = %#v", got)
	}
	if catalog.DefaultTargetRef != executable.Candidate.Target.Ref {
		t.Fatalf("default = %q, want executable %q", catalog.DefaultTargetRef, executable.Candidate.Target.Ref)
	}
}

func TestBuildCatalogSeparatesNestedModules(t *testing.T) {
	facts := syntheticFacts("module-root", "example.com/workspace", []syntheticPackage{
		{path: "example.com/workspace/client", dir: "client"},
	})
	appendCatalogModule(&facts, gofacts.ModuleFact{
		ID: "module-service", ModulePath: "example.com/workspace/service",
		ModuleDir: "services/service", Main: true,
	}, []syntheticPackage{
		{path: "example.com/workspace/service", dir: ".", executable: true, line: 11},
		{path: "example.com/workspace/service/api", dir: "api"},
	})

	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 3 {
		t.Fatalf("entries = %#v", catalog.Entries)
	}
	root := requireCatalogModuleLibrary(t, catalog, "module-root")
	nested := requireCatalogModuleLibrary(t, catalog, "module-service")
	if root.DisplayPath != "." || nested.DisplayPath != "services/service" ||
		len(nested.Candidate.Target.ModulePackages) != 1 ||
		nested.Candidate.Target.ModulePackages[0].PackageDir != "services/service/api" {
		t.Fatalf("nested module identities = %#v / %#v", root, nested)
	}
	requireCatalogPackage(t, catalog, "example.com/workspace/service")
}

func TestBuildCatalogExcludesInternalAndMainFromModuleAPI(t *testing.T) {
	facts := syntheticFacts("module-root", "example.com/workspace", []syntheticPackage{
		{path: "example.com/workspace/cmd/app", dir: "cmd/app", executable: true, line: 7},
		{path: "example.com/workspace/api", dir: "api"},
		{path: "example.com/workspace/internal/store", dir: "internal/store"},
		{path: "example.com/workspace/feature/internal/detail", dir: "feature/internal/detail"},
	})
	for _, packagePath := range []string{
		"example.com/workspace/internal/store", "example.com/workspace/feature/internal/detail",
	} {
		setPackageDeclarations(t, &facts, packagePath, false, nil)
		setPackageLoadState(t, &facts, packagePath, gofacts.PackageLoadIncomplete)
	}

	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	library := requireCatalogModuleLibrary(t, catalog, "module-root")
	if len(library.Candidate.Target.ModulePackages) != 3 {
		t.Fatalf("module scope = %#v", library.Candidate.Target.ModulePackages)
	}
	if len(library.Candidate.Target.LibraryPackages) != 1 ||
		library.Candidate.Target.LibraryPackages[0].PackagePath != "example.com/workspace/api" ||
		len(library.PackageAPIs) != 1 {
		t.Fatalf("public roots/API = %#v / %#v", library.Candidate.Target.LibraryPackages, library.PackageAPIs)
	}
	for _, pkg := range library.Candidate.Target.ModulePackages {
		if pkg.PackagePath == "example.com/workspace/cmd/app" {
			t.Fatalf("main package entered module scope: %#v", library.Candidate.Target.ModulePackages)
		}
	}
	tampered := library.Candidate.Target.Snapshot()
	tampered.LibraryPackages = []TargetPackage{{
		PackagePath: "example.com/workspace/internal/store", PackageDir: "internal/store",
	}}
	ref, err := targetRef(tampered)
	if err != nil {
		t.Fatal(err)
	}
	tampered.Ref = ref
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "internal package") {
		t.Fatalf("resealed target accepted internal public root: %v", err)
	}
}

func TestBuildCatalogOmitsUnprovableOrEmptyModuleLibrary(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*gofacts.Facts)
	}{
		{name: "incomplete external declaration scan", mutate: func(facts *gofacts.Facts) {
			setPackageDeclarations(t, facts, "example.com/workspace/api", false, nil)
		}},
		{name: "incomplete external go list facts", mutate: func(facts *gofacts.Facts) {
			setPackageLoadState(t, facts, "example.com/workspace/api", gofacts.PackageLoadIncomplete)
		}},
		{name: "missing external go list authority", mutate: func(facts *gofacts.Facts) {
			setPackageLoadCompleteness(t, facts, "example.com/workspace/api", nil)
		}},
		{name: "incomplete package inventory", mutate: func(facts *gofacts.Facts) {
			facts.Modules[0].RetainedPackagesCount--
			facts.Modules[0].Coverage.PackagesRetained--
		}},
		{name: "complete but no exports", mutate: func(facts *gofacts.Facts) {
			setPackageDeclarations(t, facts, "example.com/workspace/api", true, []gofacts.PackageDeclaration{{
				Kind: gofacts.PackageDeclarationFunc, Name: "hidden",
			}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := syntheticFacts("module-root", "example.com/workspace", []syntheticPackage{
				{path: "example.com/workspace/cmd/app", dir: "cmd/app", executable: true, line: 7},
				{path: "example.com/workspace/api", dir: "api"},
			})
			test.mutate(&facts)
			catalog, err := BuildCatalog(facts)
			if err != nil {
				t.Fatal(err)
			}
			if len(catalog.Entries) != 1 || catalog.Entries[0].Candidate.Target.Kind != KindExecutablePackage {
				t.Fatalf("catalog should retain only exact executable: %#v", catalog.Entries)
			}
		})
	}
}

func TestTargetCatalogV4IsPermutationStableAndRejectsTampering(t *testing.T) {
	facts := syntheticFacts("module-root", "example.com/workspace", []syntheticPackage{
		{path: "example.com/workspace/cmd/app", dir: "cmd/app", executable: true, line: 7},
		{path: "example.com/workspace/api", dir: "api"},
		{path: "example.com/workspace/client", dir: "client"},
	})
	setPackageDeclarations(t, &facts, "example.com/workspace/api", true, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationType, Name: "API"},
		{Kind: gofacts.PackageDeclarationFunc, Name: "Open"},
	})

	first, err := BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	permuted := facts
	permuted.Modules = reverseModules(facts.Modules)
	permuted.Packages = reversePackages(facts.Packages)
	permuted.EntrypointPackages = reverseEntrypoints(facts.EntrypointPackages)
	for index := range permuted.Packages {
		permuted.Packages[index].Declarations = reverseDeclarations(permuted.Packages[index].Declarations)
	}
	second, err := BuildCatalog(permuted)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := first.CanonicalJSON()
	secondJSON, _ := second.CanonicalJSON()
	if first.Ref != second.Ref || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("catalog changed under permutation:\n%s\n%s", firstJSON, secondJSON)
	}

	libraryIndex := catalogEntryIndex(first, KindModuleLibrary)
	for _, test := range []struct {
		name   string
		mutate func(*TargetCatalog)
	}{
		{name: "catalog ref", mutate: func(value *TargetCatalog) { value.Ref = "atc-tampered" }},
		{name: "version", mutate: func(value *TargetCatalog) { value.Version = 3 }},
		{name: "candidate key", mutate: func(value *TargetCatalog) { value.Entries[libraryIndex].Candidate.Key = "other" }},
		{name: "target module package", mutate: func(value *TargetCatalog) {
			value.Entries[libraryIndex].Candidate.Target.ModulePackages[0].PackagePath += "/drift"
		}},
		{name: "target root order", mutate: func(value *TargetCatalog) {
			roots := value.Entries[libraryIndex].Candidate.Target.LibraryPackages
			roots[0], roots[1] = roots[1], roots[0]
		}},
		{name: "API package", mutate: func(value *TargetCatalog) {
			value.Entries[libraryIndex].PackageAPIs[0].Package.PackagePath += "/drift"
		}},
		{name: "API declaration", mutate: func(value *TargetCatalog) {
			value.Entries[libraryIndex].PackageAPIs[0].Declarations[0].Name = "Invented"
		}},
		{name: "API order", mutate: func(value *TargetCatalog) {
			apis := value.Entries[libraryIndex].PackageAPIs
			apis[0], apis[1] = apis[1], apis[0]
		}},
		{name: "entry order", mutate: func(value *TargetCatalog) {
			value.Entries[0], value.Entries[1] = value.Entries[1], value.Entries[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tampered := first.Snapshot()
			test.mutate(&tampered)
			if err := tampered.Validate(); err == nil {
				t.Fatal("Validate accepted tampered catalog")
			}
		})
	}

	snapshot := first.Snapshot()
	snapshot.Entries[libraryIndex].PackageAPIs[0].Declarations[0].Name = "Mutated"
	snapshot.Entries[libraryIndex].Candidate.Target.ModulePackages[0].PackagePath = "mutated"
	if first.Entries[libraryIndex].PackageAPIs[0].Declarations[0].Name == "Mutated" ||
		first.Entries[libraryIndex].Candidate.Target.ModulePackages[0].PackagePath == "mutated" {
		t.Fatal("Snapshot shares module-library storage")
	}
}

func TestBuildCatalogRejectsInconsistentModuleRelativeDisplay(t *testing.T) {
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-addon", ModulePath: "example.com/addon", ModuleDir: "addons/addon", Main: true,
			PackagesCount: 1, RetainedPackagesCount: 1,
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/addon/cmd/app", Name: "main", ModuleID: "module-addon",
			ModulePath: "example.com/addon", PackageDir: "addons/addon/wrong", ModuleRelativeDir: "cmd/app", Locality: "local",
		}},
		EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: "example.com/addon", ImportPath: "example.com/addon/cmd/app", PackageDir: "addons/addon/wrong",
			ModuleRelativeDir: "cmd/app", ModuleDir: "addons/addon", Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: "addons/addon/cmd/app/main.go", Line: 3,
			}},
		}},
	}
	if _, err := BuildCatalog(facts); err == nil || !strings.Contains(err.Error(), "inconsistent repository-relative directory") {
		t.Fatalf("inconsistent display error = %v", err)
	}
}

func appendCatalogModule(facts *gofacts.Facts, module gofacts.ModuleFact, packages []syntheticPackage) {
	module.PackagesCount = len(packages)
	module.RetainedPackagesCount = len(packages)
	module.Coverage = gofacts.ModuleCoverage{
		State: gofacts.CoverageComplete, PackagesDiscovered: len(packages), PackagesRetained: len(packages),
	}
	facts.Modules = append(facts.Modules, module)
	for _, definition := range packages {
		packageDir, _ := catalogDisplayPath(module.ModuleDir, definition.dir)
		pkg := gofacts.PackageFact{
			CanonicalPath: definition.path, Name: packageName(definition), ModuleID: module.ID,
			ModulePath: module.ModulePath, PackageDir: packageDir, ModuleRelativeDir: definition.dir,
			DisplayPath: definition.dir, Locality: "local", LoadCompleteness: completePackageLoad(),
		}
		if !definition.executable {
			pkg.DeclarationsScanned = true
			pkg.Declarations = []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationType, Name: "Public"}}
		}
		facts.Packages = append(facts.Packages, pkg)
		if !definition.executable {
			continue
		}
		kind := definition.kind
		if kind == "" {
			kind = "unknown"
		}
		facts.EntrypointPackages = append(facts.EntrypointPackages, gofacts.Entrypoint{
			ModulePath: module.ModulePath, ImportPath: definition.path, Dir: packageDir,
			PackageDir: packageDir, ModuleRelativeDir: definition.dir, ModuleDir: module.ModuleDir,
			Kind: kind, GoFiles: []string{"main.go"}, Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: pathJoinForTest(packageDir, "main.go"), Line: definition.line,
			}},
		})
	}
	facts.PackagesCount += len(packages)
	facts.RetainedPackagesCount += len(packages)
}

func setPackageDeclarations(
	t *testing.T,
	facts *gofacts.Facts,
	packagePath string,
	scanned bool,
	declarations []gofacts.PackageDeclaration,
) {
	t.Helper()
	for index := range facts.Packages {
		if facts.Packages[index].CanonicalPath == packagePath {
			facts.Packages[index].DeclarationsScanned = scanned
			facts.Packages[index].Declarations = append([]gofacts.PackageDeclaration(nil), declarations...)
			return
		}
	}
	t.Fatalf("missing package %q", packagePath)
}

func setPackageLoadState(
	t *testing.T,
	facts *gofacts.Facts,
	packagePath string,
	state gofacts.PackageLoadState,
) {
	t.Helper()
	setPackageLoadCompleteness(t, facts, packagePath, &gofacts.PackageLoadCompleteness{
		Version: gofacts.PackageLoadCompletenessVersion,
		State:   state,
	})
}

func setPackageLoadCompleteness(
	t *testing.T,
	facts *gofacts.Facts,
	packagePath string,
	completeness *gofacts.PackageLoadCompleteness,
) {
	t.Helper()
	for index := range facts.Packages {
		if facts.Packages[index].CanonicalPath == packagePath {
			facts.Packages[index].LoadCompleteness = completeness
			return
		}
	}
	t.Fatalf("missing package %q", packagePath)
}

func pathJoinForTest(directory, name string) string {
	if directory == "." {
		return name
	}
	return directory + "/" + name
}

func requireCatalogPackage(t *testing.T, catalog TargetCatalog, packagePath string) TargetCatalogEntry {
	t.Helper()
	for _, entry := range catalog.Entries {
		if entry.Candidate.Target.PackagePath == packagePath {
			return entry
		}
	}
	t.Fatalf("catalog has no package %q: %#v", packagePath, catalog.Entries)
	return TargetCatalogEntry{}
}

func requireCatalogModuleLibrary(t *testing.T, catalog TargetCatalog, moduleID string) TargetCatalogEntry {
	t.Helper()
	for _, entry := range catalog.Entries {
		if entry.Candidate.Target.Kind == KindModuleLibrary && entry.Candidate.Target.ModuleID == moduleID {
			return entry
		}
	}
	t.Fatalf("catalog has no module library %q: %#v", moduleID, catalog.Entries)
	return TargetCatalogEntry{}
}

func catalogEntryIndex(catalog TargetCatalog, kind Kind) int {
	for index, entry := range catalog.Entries {
		if entry.Candidate.Target.Kind == kind {
			return index
		}
	}
	return -1
}

func reverseModules(values []gofacts.ModuleFact) []gofacts.ModuleFact {
	result := append([]gofacts.ModuleFact(nil), values...)
	slices.Reverse(result)
	return result
}

func reversePackages(values []gofacts.PackageFact) []gofacts.PackageFact {
	result := append([]gofacts.PackageFact(nil), values...)
	slices.Reverse(result)
	return result
}

func reverseEntrypoints(values []gofacts.Entrypoint) []gofacts.Entrypoint {
	result := append([]gofacts.Entrypoint(nil), values...)
	slices.Reverse(result)
	return result
}

func reverseDeclarations(values []gofacts.PackageDeclaration) []gofacts.PackageDeclaration {
	result := append([]gofacts.PackageDeclaration(nil), values...)
	slices.Reverse(result)
	return result
}
