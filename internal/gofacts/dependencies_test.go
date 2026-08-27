package gofacts

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
)

func TestBuildDependencyCatalogUsesExactGoListKindsAndDeduplicatesImporters(t *testing.T) {
	t.Parallel()

	rootModule := &goListModule{Path: "example.com/root", Main: true, Dir: "/repo"}
	serviceModule := &goListModule{Path: "example.com/service", Main: true, Dir: "/repo/service"}
	externalModule := &goListModule{
		Path: "corpdep", Version: "v0.0.0",
		Replace: &goListModule{Path: "../external", Dir: "/outside/external"},
	}
	loads := []dependencyPackageLoad{
		{packages: []goListPackage{
			{
				ImportPath: "example.com/root/app", Dir: "/repo/app", Name: "app", Module: rootModule,
				Imports: []string{"fmt", "corpdep/client", "example.com/root/lib", "corpdep/client"},
			},
			{ImportPath: "example.com/root/lib", Dir: "/repo/lib", Name: "lib", Module: rootModule},
			{ImportPath: "fmt", Dir: "/goroot/src/fmt", Name: "fmt", DepOnly: true, Standard: true},
			{ImportPath: "corpdep/client", Dir: "/outside/external/client", Name: "client", DepOnly: true, Module: externalModule},
		}},
		{packages: []goListPackage{
			{
				ImportPath: "example.com/service", Dir: "/repo/service", Name: "service", Module: serviceModule,
				Imports: []string{"example.com/root/lib", "corpdep/client"},
			},
			{
				ImportPath: "example.com/root/lib", Dir: "/repo/lib", Name: "lib", DepOnly: true,
				Module: &goListModule{Path: "example.com/root", Replace: &goListModule{Path: "..", Dir: "/repo"}},
			},
			{ImportPath: "corpdep/client", Dir: "/outside/external/client", Name: "client", DepOnly: true, Module: externalModule},
		}},
	}

	catalog, warnings, err := buildDependencyCatalog("/repo", loads)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %q", warnings)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Importers) != 2 || len(catalog.Dependencies) != 3 {
		t.Fatalf("catalog cardinality = %d importers, %d dependencies: %#v", len(catalog.Importers), len(catalog.Dependencies), catalog)
	}

	workspace := dependencyByPackage(t, catalog, "example.com/root/lib")
	if workspace.Kind != dependencies.KindWorkspace || workspace.ModulePath != "example.com/root" ||
		workspace.RepositoryPath != "lib" || len(workspace.ImporterRefs) != 2 || workspace.Replacement != nil {
		t.Fatalf("workspace dependency = %#v", workspace)
	}
	stdlib := dependencyByPackage(t, catalog, "fmt")
	if stdlib.Kind != dependencies.KindStdlib || stdlib.ModulePath != "" || len(stdlib.ImporterRefs) != 1 {
		t.Fatalf("stdlib dependency = %#v", stdlib)
	}
	external := dependencyByPackage(t, catalog, "corpdep/client")
	if external.Kind != dependencies.KindExternal || external.ModulePath != "corpdep" ||
		external.ModuleVersion != "v0.0.0" || external.Replacement == nil ||
		!external.Replacement.Local || external.Replacement.RepositoryPath != "" || len(external.ImporterRefs) != 2 {
		t.Fatalf("dotless external dependency = %#v", external)
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("/repo")) || bytes.Contains(encoded, []byte("/outside")) {
		t.Fatalf("dependency catalog exposed an absolute host path: %s", encoded)
	}

	reversed := append([]dependencyPackageLoad(nil), loads...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	for index := range reversed {
		reversed[index].packages = append([]goListPackage(nil), reversed[index].packages...)
		for left, right := 0, len(reversed[index].packages)-1; left < right; left, right = left+1, right-1 {
			reversed[index].packages[left], reversed[index].packages[right] = reversed[index].packages[right], reversed[index].packages[left]
		}
	}
	again, againWarnings, err := buildDependencyCatalog("/repo", reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalog, again) || !reflect.DeepEqual(warnings, againWarnings) {
		t.Fatalf("dependency catalog depends on go-list row order:\n%#v\n%#v", catalog, again)
	}
}

func TestLoadBuildsDependenciesFromOneGoListAcrossNestedModules(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	external := filepath.Join(parent, "external")
	for _, dir := range []string{
		filepath.Join(repo, "local"), filepath.Join(repo, "service"), filepath.Join(external, "client"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"go.mod": `module example.com/root

go 1.24

require corpdep v0.0.0

replace corpdep => ../external
`,
		"root.go": `package root

import (
	_ "corpdep/client"
	_ "example.com/root/local"
	_ "fmt"
)
`,
		"local/local.go": "package local\n",
		"service/go.mod": `module example.com/service

go 1.24

require example.com/root v0.0.0

replace example.com/root => ..
`,
		"service/service.go": `package service

import (
	_ "example.com/root/local"
	_ "net/http"
)
`,
	}
	fileList := make([]string, 0, len(files))
	for name, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fileList = append(fileList, name)
	}
	if err := os.WriteFile(filepath.Join(external, "go.mod"), []byte("module corpdep\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "client", "client.go"), []byte("package client\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	facts, err := loadForHost(context.Background(), repo, fileList)
	if err != nil {
		t.Fatal(err)
	}
	if facts.PackagesCount != 3 || facts.RetainedPackagesCount != 3 || len(facts.Packages) != 3 {
		t.Fatalf("dependency-only packages changed workspace package facts: %d/%d %#v", facts.RetainedPackagesCount, facts.PackagesCount, facts.Packages)
	}
	if facts.Dependencies == nil {
		t.Fatal("dependency catalog is nil")
	}
	if err := facts.Dependencies.Validate(); err != nil {
		t.Fatal(err)
	}
	workspace := dependencyByPackage(t, *facts.Dependencies, "example.com/root/local")
	if workspace.Kind != dependencies.KindWorkspace || workspace.RepositoryPath != "local" || len(workspace.ImporterRefs) != 2 {
		t.Fatalf("nested-module workspace dependency = %#v", workspace)
	}
	dotless := dependencyByPackage(t, *facts.Dependencies, "corpdep/client")
	if dotless.Kind != dependencies.KindExternal || dotless.ModulePath != "corpdep" ||
		dotless.Replacement == nil || !dotless.Replacement.Local || dotless.Replacement.RepositoryPath != "" {
		t.Fatalf("dotless external replacement = %#v", dotless)
	}
	for _, stdlibPath := range []string{"fmt", "net/http"} {
		if got := dependencyByPackage(t, *facts.Dependencies, stdlibPath); got.Kind != dependencies.KindStdlib {
			t.Fatalf("%s dependency = %#v", stdlibPath, got)
		}
	}
}

func TestParseGoListOutputKeepsDepOnlyMetadataWithoutChangingRootWarnings(t *testing.T) {
	t.Parallel()

	packages, warnings, err := parseGoListOutput(strings.NewReader(`
{"ImportPath":"corpdep/client","Name":"client","DepOnly":true,"Incomplete":true,"Error":{"Err":"dependency detail"},"Module":{"Path":"corpdep"}}
{"ImportPath":"example.com/root","Name":"root","GoFiles":["root.go"],"Incomplete":true,"DepsErrors":[{"Err":"dependency unavailable"}],"Module":{"Path":"example.com/root","Main":true}}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || !packages[0].DepOnly || len(rootGoListPackages(packages)) != 1 {
		t.Fatalf("decoded packages = %#v", packages)
	}
	if !reflect.DeepEqual(warnings, []string{
		"package example.com/root: go list facts are incomplete",
		"package example.com/root dependency: dependency unavailable",
	}) {
		t.Fatalf("root warnings = %q", warnings)
	}
}

func TestBuildDependencyCatalogMarksMissingAndBrokenImportsPartial(t *testing.T) {
	t.Parallel()

	rootModule := &goListModule{Path: "example.com/root", Main: true, Dir: "/repo"}
	catalog, warnings, err := buildDependencyCatalog("/repo", []dependencyPackageLoad{{packages: []goListPackage{
		{
			ImportPath: "example.com/root/app", Dir: "/repo/app", Name: "app", Module: rootModule,
			Imports: []string{"broken/module", "missing/module"},
		},
		{
			ImportPath: "broken/module", Name: "broken", DepOnly: true,
			Module: &goListModule{Path: "broken"}, Error: &goListError{Err: "unavailable"},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 2 || catalog.Coverage.State != dependencies.CoveragePartial ||
		catalog.Coverage.ImportsObserved != 2 || catalog.Coverage.ImportsRetained != 0 ||
		len(catalog.Coverage.Omissions) != 2 {
		t.Fatalf("partial dependency catalog = %#v, warnings %q", catalog, warnings)
	}
	reasons := map[string]dependencies.OmissionReason{}
	for _, omission := range catalog.Coverage.Omissions {
		if omission.ImporterRef == "" || omission.ImporterPackagePath != "example.com/root/app" {
			t.Fatalf("omission lost exact importer authority: %#v", omission)
		}
		reasons[omission.PackagePath] = omission.Reason
	}
	if reasons["missing/module"] != dependencies.OmissionDependencyMetadataMissing ||
		reasons["broken/module"] != dependencies.OmissionDependencyLoadUnavailable {
		t.Fatalf("omission reasons = %#v", reasons)
	}
}

func dependencyByPackage(t *testing.T, catalog dependencies.Catalog, packagePath string) dependencies.Dependency {
	t.Helper()
	for _, value := range catalog.Dependencies {
		if value.PackagePath == packagePath {
			return value
		}
	}
	t.Fatalf("dependency %q not found in %#v", packagePath, catalog.Dependencies)
	return dependencies.Dependency{}
}
