package report

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestProjectLibraryAPIRetainsRootAndEveryExportedDeclarationKind(t *testing.T) {
	facts := d281LibraryFacts(t, []gofacts.PackageFact{
		d281Package(t, "example.com/sdk", ".", []string{"api.go", "bot.go", "version.go"}, []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationVar, Name: "Version", Path: "version.go", Line: 4, Column: 5},
			{Kind: gofacts.PackageDeclarationFunc, Name: "NewBot", Path: "bot.go", Line: 7, Column: 6, ExecutableBody: true},
			{Kind: gofacts.PackageDeclarationType, Name: "Bot", Path: "bot.go", Line: 3, Column: 6},
			{Kind: gofacts.PackageDeclarationMethod, Receiver: "Bot", Name: "Raw", Path: "bot.go", Line: 15, Column: 15, ExecutableBody: true},
			{Kind: gofacts.PackageDeclarationConst, Name: "DefaultAPI", Path: "api.go", Line: 3, Column: 7},
			{Kind: gofacts.PackageDeclarationFunc, Name: "assembly", Path: "api.go", Line: 8, Column: 6},
		}),
		d281Package(t, "example.com/sdk/react", "react", []string{"react/react.go"}, []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationType, Name: "Reaction", Path: "react/react.go", Line: 11, Column: 6},
			{Kind: gofacts.PackageDeclarationFunc, Name: "React", Path: "react/react.go", Line: 5, Column: 6, ExecutableBody: true},
		}),
		d281Package(t, "example.com/sdk/internal/secret", "internal/secret", []string{"internal/secret/secret.go"}, []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationFunc, Name: "PublicSecret", Path: "internal/secret/secret.go", Line: 3, Column: 6, ExecutableBody: true},
		}),
	})
	target := d281ModuleLibraryTarget(t, facts)

	projection, err := ProjectLibraryAPI(facts, target)
	if err != nil {
		t.Fatalf("ProjectLibraryAPI: %v", err)
	}
	if projection.Version != LibraryAPIReportProjectionVersion ||
		projection.ModulePath != "example.com/sdk" || projection.ModuleDir != "." ||
		projection.TotalDeclarations != 7 || projection.ShownDeclarations != 7 ||
		len(projection.Packages) != 2 {
		t.Fatalf("projection identity/totals = %#v", projection)
	}
	root := projection.Packages[0]
	if root.PackagePath != "example.com/sdk" || root.PackageDir != "." || root.DisplayPath != "." ||
		root.TotalDeclarations != 5 || root.ShownDeclarations != 5 ||
		root.Counts != (LibraryAPICounts{Functions: 1, Methods: 1, Types: 1, Consts: 1, Vars: 1}) {
		t.Fatalf("root API package = %#v", root)
	}
	wantRoot := []LibraryAPIDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "NewBot", Path: "bot.go", Line: 7, Column: 6},
		{Kind: gofacts.PackageDeclarationMethod, Name: "Raw", Receiver: "Bot", Path: "bot.go", Line: 15, Column: 15},
		{Kind: gofacts.PackageDeclarationType, Name: "Bot", Path: "bot.go", Line: 3, Column: 6},
		{Kind: gofacts.PackageDeclarationVar, Name: "Version", Path: "version.go", Line: 4, Column: 5},
		{Kind: gofacts.PackageDeclarationConst, Name: "DefaultAPI", Path: "api.go", Line: 3, Column: 7},
	}
	if !reflect.DeepEqual(root.Declarations, wantRoot) {
		t.Fatalf("root declarations = %#v, want %#v", root.Declarations, wantRoot)
	}
	react := projection.Packages[1]
	if react.PackagePath != "example.com/sdk/react" || react.DisplayPath != "react" ||
		react.Counts != (LibraryAPICounts{Functions: 1, Types: 1}) {
		t.Fatalf("React API package = %#v", react)
	}
	for _, pkg := range projection.Packages {
		if strings.Contains(pkg.PackagePath, "/internal/") {
			t.Fatalf("internal package escaped target boundary: %#v", pkg)
		}
	}

	permuted := facts
	permuted.Packages = slices.Clone(facts.Packages)
	slices.Reverse(permuted.Packages)
	for index := range permuted.Packages {
		permuted.Packages[index].Declarations = slices.Clone(permuted.Packages[index].Declarations)
		slices.Reverse(permuted.Packages[index].Declarations)
	}
	again, err := ProjectLibraryAPI(permuted, target)
	if err != nil {
		t.Fatalf("ProjectLibraryAPI permuted: %v", err)
	}
	if !reflect.DeepEqual(projection, again) {
		t.Fatalf("package/declaration permutation changed projection")
	}
	withoutModule := facts
	withoutModule.Modules = nil
	if _, err := ProjectLibraryAPI(withoutModule, target); err == nil ||
		!strings.Contains(err.Error(), "module authority is unavailable") {
		t.Fatalf("missing selected module authority error = %v", err)
	}
}

func TestProjectLibraryAPIGlobalCapIsCanonicalAndFairAcrossPackages(t *testing.T) {
	rootDeclarations := make([]gofacts.PackageDeclaration, 4100)
	for index := range rootDeclarations {
		rootDeclarations[index] = gofacts.PackageDeclaration{
			Kind: gofacts.PackageDeclarationFunc, Name: fmt.Sprintf("F%04d", index),
			Path: "api.go", Line: index + 1, Column: 6, ExecutableBody: true,
		}
	}
	facts := d281LibraryFacts(t, []gofacts.PackageFact{
		d281Package(t, "example.com/sdk", ".", []string{"api.go"}, rootDeclarations),
		d281Package(t, "example.com/sdk/a", "a", []string{"a/a.go"}, []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationType, Name: "Alpha", Path: "a/a.go", Line: 2, Column: 6},
			{Kind: gofacts.PackageDeclarationVar, Name: "Beta", Path: "a/a.go", Line: 3, Column: 5},
		}),
		d281Package(t, "example.com/sdk/b", "b", []string{"b/b.go"}, []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationConst, Name: "Gamma", Path: "b/b.go", Line: 2, Column: 7},
		}),
	})
	target := d281ModuleLibraryTarget(t, facts)

	projection, err := ProjectLibraryAPI(facts, target)
	if err != nil {
		t.Fatalf("ProjectLibraryAPI: %v", err)
	}
	if projection.TotalDeclarations != 4103 || projection.ShownDeclarations != MaxLibraryAPIDeclarations {
		t.Fatalf("bounded totals = %d/%d", projection.TotalDeclarations, projection.ShownDeclarations)
	}
	shown := []int{
		projection.Packages[0].ShownDeclarations,
		projection.Packages[1].ShownDeclarations,
		projection.Packages[2].ShownDeclarations,
	}
	if !slices.Equal(shown, []int{4093, 2, 1}) {
		t.Fatalf("fair package allocation = %v", shown)
	}
	if projection.Packages[0].Counts.Functions != 4100 ||
		projection.Packages[1].Counts != (LibraryAPICounts{Types: 1, Vars: 1}) ||
		projection.Packages[2].Counts != (LibraryAPICounts{Consts: 1}) {
		t.Fatalf("complete package counts = %#v", projection.Packages)
	}
}

func TestReadRunDirRehydratesLibraryAPIAndAuthorizesEveryShownSource(t *testing.T) {
	facts := d281LibraryFacts(t, []gofacts.PackageFact{
		d281Package(t, "example.com/sdk", ".", []string{"api.go", "types.go"}, []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationFunc, Name: "Assembly", Path: "api.go", Line: 3, Column: 6},
			{Kind: gofacts.PackageDeclarationType, Name: "Client", Path: "types.go", Line: 2, Column: 6},
		}),
	})
	target := d281ModuleLibraryTarget(t, facts)
	runDir := t.TempDir()
	snapshot, err := json.Marshal(map[string]any{
		"repo_name": "sdk", "analysis_target": target, "go_facts": facts,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", string(snapshot))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	if data.LibraryAPI == nil || data.LibraryAPI.TotalDeclarations != 2 ||
		!slices.Equal(data.OpenablePaths, []string{"api.go", "types.go"}) {
		t.Fatalf("rehydrated projection/source authority = %#v / %v", data.LibraryAPI, data.OpenablePaths)
	}
	if err := validateLibraryAPIProjection(data.AnalysisTarget, data.LibraryAPI, data.OpenablePaths); err != nil {
		t.Fatalf("validate rehydrated projection: %v", err)
	}

	root, err := os.OpenRoot(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	persisted := *data
	persisted.repositoryGoFacts = nil
	if err := rehydrateLibraryAPIProjection(root, &persisted, true); err != nil {
		t.Fatalf("verify persisted projection: %v", err)
	}
	persisted.LibraryAPI.Packages[0].Declarations[0].Line++
	persisted.repositoryGoFacts = nil
	if err := rehydrateLibraryAPIProjection(root, &persisted, true); err == nil ||
		!strings.Contains(err.Error(), "does not match snapshot authority") {
		t.Fatalf("drifted projection error = %v", err)
	}
}

func TestRunManifestRequiresCurrentLibraryAPIAndExactSourceAuthority(t *testing.T) {
	facts := d281LibraryFacts(t, []gofacts.PackageFact{
		d281Package(t, "example.com/sdk", ".", []string{"api.go", "types.go"}, []gofacts.PackageDeclaration{
			{Kind: gofacts.PackageDeclarationFunc, Name: "Open", Path: "api.go", Line: 3, Column: 6, ExecutableBody: true},
			{Kind: gofacts.PackageDeclarationType, Name: "Client", Path: "types.go", Line: 2, Column: 6},
		}),
	})
	target := d281ModuleLibraryTarget(t, facts)
	projection, err := ProjectLibraryAPI(facts, target)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(t *testing.T, libraryAPI *LibraryAPIReportProjection, openable []string) ([]byte, RunManifest) {
		t.Helper()
		data := ReportData{
			FormatVersion: CurrentFormatVersion, AnalysisTarget: &target,
			LibraryAPI: libraryAPI, OpenablePaths: openable,
		}
		reportJSON, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		manifest := validRunManifestFixture(t)
		manifest.OpenablePaths = slices.Clone(openable)
		manifest.Components = nil
		manifest.ReportSHA256 = manifestSHA256(reportJSON)
		manifest.MaterialInputs.AnalysisTargetRef, manifest.MaterialInputs.AnalysisTargetSHA256, err =
			reportAnalysisTargetBinding(&target)
		if err != nil {
			t.Fatal(err)
		}
		return reportJSON, manifest
	}

	reportJSON, manifest := encode(t, projection, []string{"api.go", "types.go"})
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		t.Fatalf("VerifyReportJSON: %v", err)
	}
	missing, missingManifest := encode(t, nil, []string{"api.go", "types.go"})
	if err := missingManifest.VerifyReportJSON(missing); err == nil ||
		!strings.Contains(err.Error(), "has no exact public API") {
		t.Fatalf("missing library API error = %v", err)
	}
	unauthorized, unauthorizedManifest := encode(t, projection, []string{"api.go"})
	if err := unauthorizedManifest.VerifyReportJSON(unauthorized); err == nil ||
		!strings.Contains(err.Error(), "source \"types.go\" is not authorized") {
		t.Fatalf("missing exact source authority error = %v", err)
	}
}

func d281LibraryFacts(t *testing.T, packages []gofacts.PackageFact) gofacts.Facts {
	t.Helper()
	return gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: "example.com/sdk", ModuleDir: ".", Main: true,
			PackagesCount: len(packages), RetainedPackagesCount: len(packages),
			Coverage: gofacts.ModuleCoverage{
				State: gofacts.CoverageComplete, PackagesDiscovered: len(packages), PackagesRetained: len(packages),
			},
		}},
		Packages: packages,
	}
}

func d281Package(
	t *testing.T,
	packagePath,
	packageDir string,
	files []string,
	declarations []gofacts.PackageDeclaration,
) gofacts.PackageFact {
	t.Helper()
	canonical, err := gofacts.CanonicalPackageDeclarations(declarations)
	if err != nil {
		t.Fatalf("canonical declarations for %s: %v", packagePath, err)
	}
	return gofacts.PackageFact{
		CanonicalPath: packagePath, Name: packageNameForD281(packagePath),
		ModuleID: "module-root", ModulePath: "example.com/sdk",
		PackageDir: packageDir, ModuleRelativeDir: packageDir, Locality: "local",
		Files: slices.Clone(files), Declarations: canonical, DeclarationsScanned: true,
		LoadCompleteness: &gofacts.PackageLoadCompleteness{
			Version: gofacts.PackageLoadCompletenessVersion, State: gofacts.PackageLoadComplete,
		},
	}
}

func packageNameForD281(packagePath string) string {
	if packagePath == "example.com/sdk" {
		return "sdk"
	}
	position := strings.LastIndex(packagePath, "/")
	return packagePath[position+1:]
}

func d281ModuleLibraryTarget(t *testing.T, facts gofacts.Facts) analysistarget.Target {
	t.Helper()
	catalog, err := analysistarget.BuildCatalog(facts)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	for _, entry := range catalog.Entries {
		if entry.Candidate.Target.Kind == analysistarget.KindModuleLibrary {
			return entry.Candidate.Target.Snapshot()
		}
	}
	t.Fatalf("module library target absent: %#v", catalog.Entries)
	return analysistarget.Target{}
}
