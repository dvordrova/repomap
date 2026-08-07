package goadapter

import (
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

// Long-horizon program Phase 1D (Gemnasium): the same textual package import
// path under two captured modules is an explicit QUALIFIED identity (two
// module-scoped packages), never a whole-run crash. Each module keeps its own
// unit; the Atlas publishes both.
func TestProjectSameImportPathUnderTwoModulesIsQualifiedNotCrash(t *testing.T) {
	t.Parallel()
	moduleA := gofacts.ModuleFact{ID: "module-a", ModulePath: "example.com/mod-a", Main: true}
	moduleB := gofacts.ModuleFact{ID: "module-b", ModulePath: "example.com/mod-b", Main: true}
	sharedPath := "example.com/shared/core"
	input := Input{
		RepositoryName: "example.com/repo",
		Facts: gofacts.Facts{
			Modules: []gofacts.ModuleFact{moduleA, moduleB},
			Packages: []gofacts.PackageFact{
				{
					CanonicalPath: sharedPath, Name: "core", ModuleID: "module-a", ModulePath: "example.com/mod-a",
					PackageDir: "mod-a/core", ModuleRelativeDir: "core", DisplayPath: "core", Locality: "local",
					Files: []string{"mod-a/core/core.go"},
				},
				{
					CanonicalPath: sharedPath, Name: "core", ModuleID: "module-b", ModulePath: "example.com/mod-b",
					PackageDir: "mod-b/core", ModuleRelativeDir: "core", DisplayPath: "core", Locality: "local",
					Files: []string{"mod-b/core/core.go"},
				},
			},
		},
		Catalog: surfacediscovery.TriggerCatalog{},
	}
	atlas, err := Project(input)
	if err != nil {
		t.Fatalf("same import path under two modules must not crash the Atlas: %v", err)
	}
	// Both module-scoped packages publish as distinct units.
	unitCount := 0
	for _, unit := range atlas.Units {
		if unit.Kind == repositoryatlas.UnitPackage && unit.Name == sharedPath {
			unitCount++
		}
	}
	if unitCount != 2 {
		t.Fatalf("expected 2 module-scoped units for %q, got %d", sharedPath, unitCount)
	}
	// Parents are the distinct modules — qualified identity, not merged.
	parents := make(map[string]struct{})
	for _, unit := range atlas.Units {
		if unit.Kind == repositoryatlas.UnitPackage && unit.Name == sharedPath {
			parents[unit.ParentID] = struct{}{}
		}
	}
	if len(parents) != 2 {
		t.Fatalf("expected two distinct owning modules, got %#v", parents)
	}
}

// TestProjectDuplicatePackageUnderSameModuleStillMerges pins the deterministic
// merge for an exact duplicate under ONE module (Decision 235 1D) — the
// qualified-key change must not regress it.
func TestProjectDuplicatePackageUnderSameModuleMerges(t *testing.T) {
	t.Parallel()
	input := Input{
		RepositoryName: "example.com/repo",
		Facts: gofacts.Facts{
			Modules: []gofacts.ModuleFact{{ID: "module-a", ModulePath: "example.com/mod-a", Main: true}},
			Packages: []gofacts.PackageFact{
				{
					CanonicalPath: "example.com/mod-a/core", Name: "core", ModuleID: "module-a", ModulePath: "example.com/mod-a",
					PackageDir: "core", ModuleRelativeDir: "core", DisplayPath: "core", Locality: "local",
					Files: []string{"core/core.go"},
				},
				{
					CanonicalPath: "example.com/mod-a/core", Name: "core", ModuleID: "module-a", ModulePath: "example.com/mod-a",
					PackageDir: "core", ModuleRelativeDir: "core", DisplayPath: "core", Locality: "local",
					Files: []string{"core/core.go"},
				},
			},
		},
		Catalog: surfacediscovery.TriggerCatalog{},
	}
	atlas, err := Project(input)
	if err != nil {
		t.Fatalf("duplicate package under one module must merge, not fail: %v", err)
	}
	_ = evidence.Location{}
	count := 0
	for _, unit := range atlas.Units {
		if unit.Kind == repositoryatlas.UnitPackage && unit.Name == "example.com/mod-a/core" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected a single merged unit, got %d", count)
	}
}
