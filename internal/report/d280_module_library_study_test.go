package report

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestD280ModuleLibraryStudyBindsEveryPublicPackageWithoutInternalAPI(t *testing.T) {
	declaration := func(name, sourcePath string, line int) []gofacts.PackageDeclaration {
		values, err := gofacts.CanonicalPackageDeclarations([]gofacts.PackageDeclaration{{
			Kind: gofacts.PackageDeclarationFunc, Name: name,
			Path: sourcePath, Line: line, Column: 6, ExecutableBody: true,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return values
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: "example.com/sdk", ModuleDir: ".", Main: true,
			PackagesCount: 3, RetainedPackagesCount: 3,
			Coverage: gofacts.ModuleCoverage{
				State: gofacts.CoverageComplete, PackagesDiscovered: 3, PackagesRetained: 3,
			},
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/sdk", Name: "sdk", ModuleID: "module-root",
				ModulePath: "example.com/sdk", PackageDir: ".", ModuleRelativeDir: ".", Locality: "local",
				Files: []string{"root.go"}, Declarations: declaration("NewRoot", "root.go", 3), DeclarationsScanned: true,
				LoadCompleteness: completeReportPackageLoad(),
			},
			{
				CanonicalPath: "example.com/sdk/sub", Name: "sub", ModuleID: "module-root",
				ModulePath: "example.com/sdk", PackageDir: "sub", ModuleRelativeDir: "sub", Locality: "local",
				Files: []string{"sub/sub.go"}, Declarations: declaration("NewSub", "sub/sub.go", 5), DeclarationsScanned: true,
				LoadCompleteness: completeReportPackageLoad(),
			},
			{
				CanonicalPath: "example.com/sdk/internal/secret", Name: "secret", ModuleID: "module-root",
				ModulePath: "example.com/sdk", PackageDir: "internal/secret", ModuleRelativeDir: "internal/secret", Locality: "local",
				Files: []string{"internal/secret/secret.go"}, Declarations: declaration("NewSecret", "internal/secret/secret.go", 7), DeclarationsScanned: true,
				LoadCompleteness: completeReportPackageLoad(),
			},
		},
	}
	catalog, err := analysistarget.BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 1 || catalog.Entries[0].Candidate.Target.Kind != analysistarget.KindModuleLibrary {
		t.Fatalf("module target catalog = %#v", catalog.Entries)
	}
	target := catalog.Entries[0].Candidate.Target.Snapshot()
	atlas := repositoryatlas.Atlas{Version: repositoryatlas.Version, Units: []repositoryatlas.Unit{
		{ID: "unit-repository", Kind: repositoryatlas.UnitRepository, Name: "repository"},
		{ID: "module-root", Kind: repositoryatlas.UnitModule, ParentID: "unit-repository", Name: "example.com/sdk"},
		{ID: "unit-root", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "example.com/sdk"},
		{ID: "unit-sub", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "example.com/sdk/sub"},
		{ID: "unit-secret", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "example.com/sdk/internal/secret"},
	}}
	data := &ReportData{AnalysisTarget: &target, RepositoryAtlas: &atlas, repositoryGoFacts: &facts}
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if input.AnalysisTargetRoot == nil || len(input.AnalysisTargetRoot.Packages) != 2 {
		t.Fatalf("module root scope = %#v", input.AnalysisTargetRoot)
	}
	labels := make([]string, 0, len(input.ReadingTargets))
	units := make([]string, 0, len(input.ReadingTargets))
	for _, reading := range input.ReadingTargets {
		labels = append(labels, reading.Label)
		units = append(units, reading.PrincipalRefs[0].ID)
	}
	slices.Sort(labels)
	slices.Sort(units)
	if !slices.Equal(labels, []string{"NewRoot", "NewSub"}) ||
		!slices.Equal(units, []string{"unit-root", "unit-sub"}) {
		t.Fatalf("module library readings labels=%v units=%v", labels, units)
	}
	if _, err := atlasstudy.Compile(input); err != nil {
		t.Fatalf("compile module library Study: %v", err)
	}
	collectOpenablePaths(data)
	if !slices.Contains(data.OpenablePaths, "root.go") || !slices.Contains(data.OpenablePaths, "sub/sub.go") ||
		slices.Contains(data.OpenablePaths, "internal/secret/secret.go") {
		t.Fatalf("module library source authority = %v", data.OpenablePaths)
	}
}
