package analysistarget

import (
	"bytes"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestBuildCatalogRepomapInventoryAndDefaultMatchResolve(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "github.com/dvordrova/repomap",
		[]syntheticPackage{
			{path: "github.com/dvordrova/repomap/cmd/quality-evaluate", dir: "cmd/quality-evaluate", executable: true, kind: "tool", line: 16},
			{path: "github.com/dvordrova/repomap/cmd/repomap", dir: "cmd/repomap", executable: true, line: 43},
			{path: "github.com/dvordrova/repomap/internal/report", dir: "internal/report"},
			{path: "github.com/dvordrova/repomap/pkg/client", dir: "pkg/client"},
		},
	)
	appendCatalogModule(
		&facts,
		gofacts.ModuleFact{
			ID: "module-beego-testdata", ModulePath: "example.com/beegoapp",
			ModuleDir: "internal/surfacediscovery/testdata/beego", Main: true,
		},
		[]syntheticPackage{{
			path: "example.com/beegoapp", dir: ".", executable: true, line: 7,
		}},
	)

	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(catalog.Entries) != 5 {
		t.Fatalf("entries = %d, want 5", len(catalog.Entries))
	}

	wantDisplays := map[string]string{
		"github.com/dvordrova/repomap/cmd/quality-evaluate": "cmd/quality-evaluate",
		"github.com/dvordrova/repomap/cmd/repomap":          "cmd/repomap",
		"github.com/dvordrova/repomap/internal/report":      "internal/report",
		"github.com/dvordrova/repomap/pkg/client":           "pkg/client",
		"example.com/beegoapp":                              "internal/surfacediscovery/testdata/beego",
	}
	for packagePath, wantDisplay := range wantDisplays {
		entry := requireCatalogPackage(t, catalog, packagePath)
		if entry.DisplayPath != wantDisplay {
			t.Fatalf("display for %q = %q, want %q", packagePath, entry.DisplayPath, wantDisplay)
		}
		if entry.Candidate.Target.ModuleID == "" || entry.Candidate.Target.PackagePath != packagePath {
			t.Fatalf("entry lost composite identity: %#v", entry.Candidate.Target)
		}
	}

	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Selected == nil {
		t.Fatalf("Resolve selected no target: %#v", resolution)
	}
	if catalog.DefaultTargetRef != resolution.Selected.Ref {
		t.Fatalf("catalog default = %q, Resolve selected = %q", catalog.DefaultTargetRef, resolution.Selected.Ref)
	}
	if got := requireCatalogPackage(t, catalog, "github.com/dvordrova/repomap/cmd/repomap").Candidate.Target.Ref; got != catalog.DefaultTargetRef {
		t.Fatalf("repomap entry ref = %q, default = %q", got, catalog.DefaultTargetRef)
	}
}

func TestBuildCatalogIncludesRootAndNestedModuleLibraries(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{
			{path: "example.com/workspace", dir: "."},
			{path: "example.com/workspace/pkg/client", dir: "pkg/client"},
			{path: "example.com/workspace/internal/store", dir: "internal/store"},
		},
	)
	appendCatalogModule(
		&facts,
		gofacts.ModuleFact{
			ID: "module-service", ModulePath: "example.com/workspace/service",
			ModuleDir: "services/service", Main: true,
		},
		[]syntheticPackage{
			{path: "example.com/workspace/service", dir: ".", executable: true, line: 11},
			{path: "example.com/workspace/service/api", dir: "api"},
		},
	)

	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	want := map[string]string{
		"example.com/workspace":                ".",
		"example.com/workspace/pkg/client":     "pkg/client",
		"example.com/workspace/internal/store": "internal/store",
		"example.com/workspace/service":        "services/service",
		"example.com/workspace/service/api":    "services/service/api",
	}
	for packagePath, displayPath := range want {
		entry := requireCatalogPackage(t, catalog, packagePath)
		if entry.DisplayPath != displayPath {
			t.Fatalf("display for %q = %q, want %q", packagePath, entry.DisplayPath, displayPath)
		}
	}
	if requireCatalogPackage(t, catalog, "example.com/workspace").Candidate.Target.Kind != KindLibraryPackage {
		t.Fatal("root package is not a library target")
	}
	if requireCatalogPackage(t, catalog, "example.com/workspace/service").Candidate.Target.Kind != KindExecutablePackage {
		t.Fatal("nested module executable is not an executable target")
	}

	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Selected == nil || catalog.DefaultTargetRef != resolution.Selected.Ref {
		t.Fatalf("catalog/Resolve default mismatch: catalog=%q resolution=%#v", catalog.DefaultTargetRef, resolution)
	}
	if resolution.Selected.PackagePath != "example.com/workspace" {
		t.Fatalf("default package = %q", resolution.Selected.PackagePath)
	}
}

func TestBuildCatalogAmbiguousResolutionHasNoDefault(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{
			{path: "example.com/workspace/cmd/api", dir: "cmd/api", executable: true, line: 10},
			{path: "example.com/workspace/cmd/worker", dir: "cmd/worker", executable: true, line: 20},
		},
	)
	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Selected != nil || resolution.State != ResolutionAmbiguous {
		t.Fatalf("resolution = %#v", resolution)
	}
	if catalog.DefaultTargetRef != "" {
		t.Fatalf("ambiguous catalog default = %q", catalog.DefaultTargetRef)
	}
}

func TestBuildCatalogUnavailableResolutionHasNoDefault(t *testing.T) {
	facts := gofacts.Facts{}
	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Selected != nil || resolution.State != ResolutionUnavailable {
		t.Fatalf("resolution = %#v", resolution)
	}
	if catalog.DefaultTargetRef != "" {
		t.Fatalf("unavailable catalog default = %q", catalog.DefaultTargetRef)
	}
}

func TestBuildCatalogIsPermutationStable(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{
			{path: "example.com/workspace", dir: "."},
			{path: "example.com/workspace/cmd/workspace", dir: "cmd/workspace", executable: true, line: 18},
			{path: "example.com/workspace/pkg/client", dir: "pkg/client"},
		},
	)
	appendCatalogModule(
		&facts,
		gofacts.ModuleFact{ID: "module-addon", ModulePath: "example.com/addon", ModuleDir: "addons/addon", Main: true},
		[]syntheticPackage{{path: "example.com/addon", dir: "."}, {path: "example.com/addon/api", dir: "api"}},
	)

	first, err := BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	permuted := facts
	permuted.Modules = reverseModules(facts.Modules)
	permuted.Packages = reversePackages(facts.Packages)
	permuted.EntrypointPackages = reverseEntrypoints(facts.EntrypointPackages)
	second, err := BuildCatalog(permuted)
	if err != nil {
		t.Fatalf("BuildCatalog permuted: %v", err)
	}
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatalf("first CanonicalJSON: %v", err)
	}
	secondJSON, err := second.CanonicalJSON()
	if err != nil {
		t.Fatalf("second CanonicalJSON: %v", err)
	}
	if first.Ref != second.Ref || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("catalog changed under permutation:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestTargetCatalogValidationRejectsTampering(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{
			{path: "example.com/workspace", dir: "."},
			{path: "example.com/workspace/pkg/client", dir: "pkg/client"},
		},
	)
	facts.Packages[0].DeclarationsScanned = true
	facts.Packages[0].Declarations = []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationType, Name: "Workspace"}}
	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*TargetCatalog)
	}{
		{name: "ref", mutate: func(value *TargetCatalog) { value.Ref = "atc-tampered" }},
		{name: "default", mutate: func(value *TargetCatalog) { value.DefaultTargetRef = "at-missing" }},
		{name: "display", mutate: func(value *TargetCatalog) { value.Entries[0].DisplayPath = "elsewhere" }},
		{name: "candidate key", mutate: func(value *TargetCatalog) { value.Entries[0].Candidate.Key = "other" }},
		{name: "symbol", mutate: func(value *TargetCatalog) { value.Entries[0].Symbols[0].Name = "Invented" }},
		{name: "scan authority", mutate: func(value *TargetCatalog) { value.Entries[0].DeclarationsScanned = false }},
		{name: "target identity", mutate: func(value *TargetCatalog) { value.Entries[0].Candidate.Target.ModuleID = "other" }},
		{name: "order", mutate: func(value *TargetCatalog) {
			value.Entries[0], value.Entries[1] = value.Entries[1], value.Entries[0]
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := catalog.Snapshot()
			test.mutate(&tampered)
			if err := tampered.Validate(); err == nil {
				t.Fatal("Validate accepted tampered catalog")
			}
		})
	}

	snapshot := catalog.Snapshot()
	snapshot.Entries[0].Candidate.Target.Roots = append(snapshot.Entries[0].Candidate.Target.Roots, Root{Path: "mutated.go", Line: 1})
	if len(catalog.Entries[0].Candidate.Target.Roots) != 0 {
		t.Fatal("Snapshot shares target root storage")
	}
}

func TestBuildCatalogFiltersLibraryAPIAndKeepsExecutableDeclarations(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{
			{path: "example.com/workspace/cmd/app", dir: "cmd/app", executable: true, line: 10},
			{path: "example.com/workspace/pkg/client", dir: "pkg/client"},
		},
	)
	all := []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "Exported", Path: "api.go", Line: 1, Column: 1},
		{Kind: gofacts.PackageDeclarationFunc, Name: "internal", Path: "api.go", Line: 2, Column: 1},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Client", Name: "Do", Path: "api.go", Line: 3, Column: 1},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Client", Name: "debug", Path: "api.go", Line: 4, Column: 1},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "hidden", Name: "Exported", Path: "api.go", Line: 5, Column: 1},
		{Kind: gofacts.PackageDeclarationType, Name: "Client", Path: "api.go", Line: 6, Column: 1},
		{Kind: gofacts.PackageDeclarationType, Name: "hidden", Path: "api.go", Line: 7, Column: 1},
	}
	for index := range facts.Packages {
		facts.Packages[index].DeclarationsScanned = true
		facts.Packages[index].Declarations = append([]gofacts.PackageDeclaration(nil), all...)
	}
	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	executable := requireCatalogPackage(t, catalog, "example.com/workspace/cmd/app")
	library := requireCatalogPackage(t, catalog, "example.com/workspace/pkg/client")
	if len(executable.Symbols) != len(all) {
		t.Fatalf("executable symbols = %#v, want all %#v", executable.Symbols, all)
	}
	wantLibrary := []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "Exported"},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Client", Name: "Do"},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "hidden", Name: "Exported"},
		{Kind: gofacts.PackageDeclarationType, Name: "Client"},
	}
	if !slices.Equal(library.Symbols, wantLibrary) {
		t.Fatalf("library symbols = %#v, want %#v", library.Symbols, wantLibrary)
	}
	for _, entry := range catalog.Entries {
		for _, symbol := range entry.Symbols {
			if symbol.Path != "" || symbol.Line != 0 || symbol.Column != 0 {
				t.Fatalf("names-only catalog leaked declaration location: %#v", symbol)
			}
		}
	}
}

func TestTargetCatalogV3RejectsPriorV2Identity(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{{path: "example.com/workspace", dir: "."}},
	)
	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Version != 3 {
		t.Fatalf("catalog version = %d, want 3", catalog.Version)
	}
	prior := catalog.Snapshot()
	prior.Version = 2
	if err := prior.Validate(); err == nil {
		t.Fatal("Validate accepted prior v2 catalog identity")
	}
}

func TestTargetCatalogValidationRejectsResealedStructuralTampering(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{{path: "example.com/workspace", dir: "."}},
	)
	catalog, err := BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}

	t.Run("package outside nested module", func(t *testing.T) {
		tampered := catalog.Snapshot()
		tampered.Entries[0].Candidate.MainModule = false
		tampered.Entries[0].Candidate.Target.ModuleDir = "services/api"
		tampered.Entries[0].Candidate.Target.PackageDir = "elsewhere"
		tampered.Entries[0].DisplayPath = "elsewhere"
		resealCatalogForTest(t, &tampered)
		if err := tampered.Validate(); err == nil {
			t.Fatal("Validate accepted a package outside its nested module")
		}
	})

	t.Run("nested module marked root", func(t *testing.T) {
		tampered := catalog.Snapshot()
		tampered.Entries[0].Candidate.Target.ModuleDir = "services/api"
		tampered.Entries[0].Candidate.Target.PackageDir = "services/api"
		tampered.Entries[0].DisplayPath = "services/api"
		resealCatalogForTest(t, &tampered)
		if err := tampered.Validate(); err == nil {
			t.Fatal("Validate accepted a nested package as part of the root analysis module")
		}
	})
}

func TestBuildCatalogRejectsInconsistentModuleRelativeDisplay(t *testing.T) {
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-addon", ModulePath: "example.com/addon", ModuleDir: "addons/addon", Main: true,
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/addon", Name: "addon", ModuleID: "module-addon",
			ModulePath: "example.com/addon", PackageDir: ".", ModuleRelativeDir: ".", Locality: "local",
		}},
	}
	if _, err := BuildCatalog(facts); err == nil {
		t.Fatal("BuildCatalog accepted display identity inconsistent with module layout")
	}
}

func appendCatalogModule(facts *gofacts.Facts, module gofacts.ModuleFact, packages []syntheticPackage) {
	facts.Modules = append(facts.Modules, module)
	for _, definition := range packages {
		packageDir, _ := catalogDisplayPath(module.ModuleDir, definition.dir)
		facts.Packages = append(facts.Packages, gofacts.PackageFact{
			CanonicalPath: definition.path, Name: packageName(definition), ModuleID: module.ID,
			ModulePath: module.ModulePath, PackageDir: packageDir, ModuleRelativeDir: definition.dir,
			DisplayPath: definition.dir, Locality: "local",
		})
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
}

func pathJoinForTest(directory, name string) string {
	if directory == "." {
		return name
	}
	return directory + "/" + name
}

func resealCatalogForTest(t *testing.T, catalog *TargetCatalog) {
	t.Helper()
	for index := range catalog.Entries {
		target := &catalog.Entries[index].Candidate.Target
		ref, err := targetRef(*target)
		if err != nil {
			t.Fatalf("targetRef: %v", err)
		}
		target.Ref = ref
		catalog.Entries[index].Candidate.Key = candidateKey(target.ModulePath, target.ModuleDir, target.PackagePath)
	}
	catalog.DefaultTargetRef = ""
	if target, ok := catalogDefault(catalog.Entries); ok {
		catalog.DefaultTargetRef = target.Ref
	}
	ref, err := targetCatalogRef(*catalog)
	if err != nil {
		t.Fatalf("targetCatalogRef: %v", err)
	}
	catalog.Ref = ref
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

func reverseModules(values []gofacts.ModuleFact) []gofacts.ModuleFact {
	result := append([]gofacts.ModuleFact(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func reversePackages(values []gofacts.PackageFact) []gofacts.PackageFact {
	result := append([]gofacts.PackageFact(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func reverseEntrypoints(values []gofacts.Entrypoint) []gofacts.Entrypoint {
	result := append([]gofacts.Entrypoint(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}
