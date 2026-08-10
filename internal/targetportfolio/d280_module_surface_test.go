package targetportfolio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestD280TelebotIsOneModuleLibraryWithPackageQualifiedAPI(t *testing.T) {
	const modulePath = "gopkg.in/telebot.v3"
	packages := []gofacts.PackageFact{
		d280LibraryPackage("telebot", modulePath, ".", ".", "telebot", "NewBot"),
		d280LibraryPackage("telebot", modulePath, ".", "layout", "layout", "Default"),
		d280LibraryPackage("telebot", modulePath, ".", "middleware", "middleware", "Recover"),
		d280LibraryPackage("telebot", modulePath, ".", "react", "react", "New"),
		d280LibraryPackage("telebot", modulePath, ".", "internal/token", "token", "ExportedButInternal"),
	}
	facts := gofacts.Facts{
		Modules:  []gofacts.ModuleFact{d280Module("telebot", modulePath, ".", len(packages), true)},
		Packages: packages,
	}
	catalog := mustCatalog(t, facts)
	if len(catalog.Entries) != 1 || catalog.Entries[0].Candidate.Target.Kind != analysistarget.KindModuleLibrary {
		t.Fatalf("Telebot catalog = %#v, want one module library", catalog.Entries)
	}

	compilation, err := Compile("telebot", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.Request.Targets) != 1 {
		t.Fatalf("ordinary targets = %d, want one", len(compilation.Request.Targets))
	}
	target := compilation.Request.Targets[0]
	if target.DisplayPath != "." || target.Kind != TargetLibrary {
		t.Fatalf("Telebot target = %#v", target)
	}
	if got := d280PackagePaths(target.Packages); !slices.Equal(got, []string{".", "layout", "middleware", "react"}) {
		t.Fatalf("package-qualified API groups = %#v", got)
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{modulePath, "internal/token", "ExportedButInternal", `"package_path"`, `"line"`, `"column"`} {
		if bytes.Contains(wire, []byte(forbidden)) {
			t.Fatalf("private/internal material %q leaked in %s", forbidden, wire)
		}
	}

	ref := target.Ref
	raw, err := json.Marshal(Response{
		Version: ResultVersion, RequestRef: compilation.Request.RequestRef,
		DefaultRef: ref, TargetRefs: []string{ref},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := ResolveResponse(compilation, raw)
	if err != nil || len(selection.Targets) != 1 || selection.Targets[0].DisplayPath != "." {
		t.Fatalf("Telebot selection = %#v, %v", selection, err)
	}
	selection.Targets[0].PackageAPIs[0].Declarations[0].Name = "Mutated"
	if compilationAuthorityHasSymbol(compilation, "Mutated") {
		t.Fatal("restored module-library API mutation changed private compilation authority")
	}
}

func TestD280MobyPackageFactsCollapseToMainsAndThreeModuleLibraries(t *testing.T) {
	facts := d280MobyFacts()
	if len(facts.Packages) != 351 || len(facts.Modules) != 3 || len(facts.EntrypointPackages) != 11 {
		t.Fatalf("Moby fixture = packages %d modules %d mains %d", len(facts.Packages), len(facts.Modules), len(facts.EntrypointPackages))
	}
	catalog := mustCatalog(t, facts)
	var executableCount, libraryCount int
	for _, entry := range catalog.Entries {
		switch entry.Candidate.Target.Kind {
		case analysistarget.KindExecutablePackage:
			executableCount++
		case analysistarget.KindModuleLibrary:
			libraryCount++
		default:
			t.Fatalf("unexpected catalog target kind %q", entry.Candidate.Target.Kind)
		}
	}
	if executableCount != 11 || libraryCount != 3 || len(catalog.Entries) != 14 {
		t.Fatalf("collapsed catalog = %d executables + %d libraries = %d", executableCount, libraryCount, len(catalog.Entries))
	}

	compilation, err := Compile("moby", catalog)
	if err != nil {
		t.Fatalf("Compile Moby-shaped surface: %v", err)
	}
	if len(compilation.Request.Targets) != 14 {
		t.Fatalf("provider targets = %d, want catalog's 14 product surfaces", len(compilation.Request.Targets))
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) > MaxRequestBytes {
		t.Fatalf("Moby-shaped request bytes = %d, cap %d", len(wire), MaxRequestBytes)
	}
	for _, forbidden := range []string{
		"github.com/moby/moby", "github.com/moby/api", "github.com/moby/buildkit",
		"internal/hidden", "ExportedInternal", `"module_path"`, `"package_path"`, `"roots"`, ".go\"",
	} {
		if bytes.Contains(wire, []byte(forbidden)) {
			t.Fatalf("private Moby material %q leaked in bounded wire", forbidden)
		}
	}

	permuted := facts
	permuted.Modules = slices.Clone(facts.Modules)
	permuted.Packages = slices.Clone(facts.Packages)
	permuted.EntrypointPackages = slices.Clone(facts.EntrypointPackages)
	slices.Reverse(permuted.Modules)
	slices.Reverse(permuted.Packages)
	slices.Reverse(permuted.EntrypointPackages)
	for index := range permuted.Packages {
		permuted.Packages[index].Declarations = slices.Clone(permuted.Packages[index].Declarations)
		slices.Reverse(permuted.Packages[index].Declarations)
	}
	permutedCompilation, err := Compile("moby", mustCatalog(t, permuted))
	if err != nil {
		t.Fatal(err)
	}
	permutedWire, _ := ProviderVisibleJSON(permutedCompilation)
	if compilation.CatalogRef != permutedCompilation.CatalogRef || !bytes.Equal(wire, permutedWire) {
		t.Fatal("Moby module-surface request changed under facts/declaration permutation")
	}

	tampered := compilation
	for targetIndex := range tampered.Request.Targets {
		if tampered.Request.Targets[targetIndex].Kind == TargetLibrary {
			tampered.Request.Targets[targetIndex].Packages[0].DisplayPath = "invented/package"
			if _, err := ProviderVisibleJSON(tampered); err == nil {
				t.Fatal("provider boundary accepted package-group display tampering")
			}
			return
		}
	}
	t.Fatal("Moby fixture has no library target to tamper")
}

func d280MobyFacts() gofacts.Facts {
	type moduleSpec struct {
		id, modulePath, moduleDir string
		count, mains              int
		main                      bool
	}
	specs := []moduleSpec{
		{id: "root", modulePath: "github.com/moby/moby", moduleDir: ".", count: 340, mains: 9, main: true},
		{id: "api", modulePath: "github.com/moby/api", moduleDir: "api", count: 6, mains: 1},
		{id: "buildkit", modulePath: "github.com/moby/buildkit", moduleDir: "builder/builder-next", count: 5, mains: 1},
	}
	result := gofacts.Facts{}
	for _, spec := range specs {
		result.Modules = append(result.Modules, d280Module(spec.id, spec.modulePath, spec.moduleDir, spec.count, spec.main))
		for index := 0; index < spec.count; index++ {
			var relativeDir string
			switch {
			case index < spec.mains:
				relativeDir = fmt.Sprintf("cmd/product-%02d", index)
			case index == spec.mains:
				relativeDir = "internal/hidden"
			case index == spec.mains+1:
				relativeDir = "."
			default:
				relativeDir = fmt.Sprintf("pkg/surface-%03d", index)
			}
			name := strings.ReplaceAll(relativeDir, "/", "_")
			if relativeDir == "." {
				name = "moduleapi"
			}
			pkg := d280LibraryPackage(spec.id, spec.modulePath, spec.moduleDir, relativeDir, name, fmt.Sprintf("Exported%03d", index))
			if index < spec.mains {
				pkg.Name = "main"
				pkg.Declarations = []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "main"}}
				result.EntrypointPackages = append(result.EntrypointPackages, gofacts.Entrypoint{
					ModulePath: spec.modulePath, ImportPath: pkg.CanonicalPath,
					PackageDir: pkg.PackageDir, ModuleRelativeDir: relativeDir, ModuleDir: spec.moduleDir,
					Kind: "primary_binary", Anchors: []gofacts.EntrypointAnchor{{
						Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
						Path: d280Join(spec.moduleDir, relativeDir, "main.go"), Line: 10 + index,
					}},
				})
			}
			if relativeDir == "internal/hidden" {
				pkg.Declarations = []gofacts.PackageDeclaration{{Kind: gofacts.PackageDeclarationFunc, Name: "ExportedInternal"}}
			}
			result.Packages = append(result.Packages, pkg)
		}
	}
	return result
}

func d280Module(id, modulePath, moduleDir string, packageCount int, main bool) gofacts.ModuleFact {
	return gofacts.ModuleFact{
		ID: id, ModulePath: modulePath, ModuleDir: moduleDir, Main: main,
		PackagesCount: packageCount, RetainedPackagesCount: packageCount,
		Coverage: gofacts.ModuleCoverage{
			State: gofacts.CoverageComplete, PackagesDiscovered: packageCount, PackagesRetained: packageCount,
		},
	}
}

func d280LibraryPackage(
	moduleID, modulePath, moduleDir, relativeDir, name, exportedFunc string,
) gofacts.PackageFact {
	packageDir := relativeDir
	if moduleDir != "." {
		packageDir = d280Join(moduleDir, relativeDir)
	}
	canonicalPath := modulePath
	if relativeDir != "." {
		canonicalPath += "/" + relativeDir
	}
	return gofacts.PackageFact{
		CanonicalPath: canonicalPath, Name: name, ModuleID: moduleID, ModulePath: modulePath,
		PackageDir: packageDir, ModuleRelativeDir: relativeDir, DisplayPath: packageDir,
		Locality: "local", DeclarationsScanned: true, LoadCompleteness: completePackageLoadForTest(),
		Declarations: []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationFunc, Name: exportedFunc},
			{Kind: gofacts.PackageDeclarationFunc, Name: "hidden"},
		},
	}
}

func d280Join(parts ...string) string {
	var result []string
	for _, part := range parts {
		if part != "." && part != "" {
			result = append(result, part)
		}
	}
	return strings.Join(result, "/")
}

func d280PackagePaths(packages []PackageSymbols) []string {
	result := make([]string, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, pkg.DisplayPath)
	}
	return result
}
