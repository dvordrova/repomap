package main

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestD277ThemeShapeAndEncodedScoutKeepUsefulSelectedLibraryFrontier(t *testing.T) {
	input := d277ThemeLibraryInput(t, 40)
	shaped, closure, err := shapeThemeStudyCompileInput(input)
	if err != nil {
		t.Fatalf("shapeThemeStudyCompileInput: %v", err)
	}
	if closure.RetainedUnits != 3 || len(shaped.Atlas.Units) != 3 {
		t.Fatalf("selected package ancestor closure = %#v / %#v", closure, shaped.Atlas.Units)
	}
	product, err := atlasstudy.Compile(shaped)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	providerInput, err := atlasstudy.SelectAnalysisTargetRootFrontier(shaped)
	if err != nil {
		t.Fatal(err)
	}
	seeds := themeSeedSpecsFromInput(providerInput)
	if len(seeds) != 32 || seeds[0].Symbol != "NewBot" || seeds[1].Symbol != "Newline" ||
		seeds[2].Symbol != "Open" || !d277TwoReceiverFamilies(seeds[3].Symbol, seeds[4].Symbol) {
		t.Fatalf("Scout seed order = %#v", seeds[:min(6, len(seeds))])
	}
	reader := func(string, int, int) ([]string, error) { return []string{"0123456789"}, nil }
	packs, err := themestudy.BuildSeedPacks(seeds, 0, 50, 1, 64, reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs.Packs) != 5 || len(packs.Omissions) != 1 ||
		packs.Packs[0].Seed.Symbol != "NewBot" ||
		!d277TwoReceiverFamilies(packs.Packs[3].Seed.Symbol, packs.Packs[4].Seed.Symbol) {
		t.Fatalf("bounded Scout packs = %#v omissions=%#v", packs.Packs, packs.Omissions)
	}
	vocabulary := themestudy.BuildFileVocabulary([]string{"api.go", "constructors.go", "methods.go"}, 0, nil)
	request, err := themestudy.CompileScout(
		themestudy.LanguageEnglish, vocabulary, packs,
		themeScoutContext(product, "library", themeSpanAnchorRefsFromPacks(packs)), "",
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := themestudy.EncodeScoutRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := themestudy.DecodeScoutRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SeedPacks.Packs[0].Seed.Ref != "a1" ||
		decoded.SeedPacks.Packs[0].Seed.Symbol != "NewBot" {
		t.Fatalf("encoded Scout lost useful first root: %#v", decoded.SeedPacks.Packs)
	}

	permuted := input
	permuted.ReadingTargets = append([]atlasstudy.ReadingTarget(nil), input.ReadingTargets...)
	permuted.ReadingSupports = append([]atlasstudy.ReadingSupport(nil), input.ReadingSupports...)
	permuted.RouteSpans = append([]atlasstudy.RouteSpan(nil), input.RouteSpans...)
	slices.Reverse(permuted.ReadingTargets)
	slices.Reverse(permuted.ReadingSupports)
	slices.Reverse(permuted.RouteSpans)
	permuted, _, err = shapeThemeStudyCompileInput(permuted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := atlasstudy.Compile(permuted); err != nil {
		t.Fatal(err)
	}
	permuted, err = atlasstudy.SelectAnalysisTargetRootFrontier(permuted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seeds, themeSeedSpecsFromInput(permuted)) {
		t.Fatal("live/rebuild public API seed frontier changed under input permutation")
	}
}

func d277TwoReceiverFamilies(left, right string) bool {
	leftReceiver, _, leftOK := strings.Cut(left, ".")
	rightReceiver, _, rightOK := strings.Cut(right, ".")
	return leftOK && rightOK && leftReceiver != rightReceiver &&
		(leftReceiver == "Bot" || leftReceiver == "File") &&
		(rightReceiver == "Bot" || rightReceiver == "File")
}

func TestD277ThemeShapeRejectsArbitraryUnitPrincipal(t *testing.T) {
	input := d277ThemeLibraryInput(t, 4)
	input.Atlas.Units = append(input.Atlas.Units, repositoryatlas.Unit{
		ID: "unit-other", Kind: repositoryatlas.UnitPackage,
		ParentID: "module-root", Name: "example.com/library/other",
	})
	input.ReadingTargets[0].PrincipalRefs[0].ID = "unit-other"
	if _, _, err := shapeThemeStudyCompileInput(input); err == nil ||
		!strings.Contains(err.Error(), "arbitrary package Unit") {
		t.Fatalf("shape arbitrary Unit error = %v", err)
	}
}

func TestD277TypesOnlyLibraryAcceptsExactZeroRootAuthorityBeforeProvider(t *testing.T) {
	input := d277ThemeLibraryInput(t, 4)
	input.ReadingTargets = nil
	input.ReadingSupports = nil
	input.RouteSpans = nil
	target := input.AnalysisTargetRoot.AnalysisTarget
	roots := analysistarget.TargetRoots{TargetRef: target.Ref}
	if err := validateThemeLibraryRootLocators(input, target, roots); err != nil {
		t.Fatalf("exact zero/zero library roots: %v", err)
	}
	if _, err := atlasstudy.Compile(input); err == nil ||
		!strings.Contains(err.Error(), "no observed support roles") {
		t.Fatalf("types-only provider-free compile outcome = %v", err)
	}
}

func TestRunThemeStudyRejectsMissingLiveLibraryTargetBeforeDereference(t *testing.T) {
	input := d277ThemeLibraryInput(t, 1)
	target := input.AnalysisTargetRoot.AnalysisTarget.Snapshot()
	_, err := runThemeStudyForRun(
		context.Background(),
		&report.ReportData{AnalysisTarget: &target},
		nil,
		nil,
		nil,
		t.TempDir(),
		t.TempDir(),
		t.TempDir(),
		modelresearch.RepositoryContext{},
		modelresearch.DefaultPolicy(),
		false,
		true,
		themestudy.LanguageEnglish,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "selected library root authority is unavailable") {
		t.Fatalf("runThemeStudyForRun missing live target error = %v", err)
	}
}

func d277ThemeLibraryInput(t *testing.T, count int) atlasstudy.Input {
	t.Helper()
	resolution, err := analysistarget.Resolve(gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: "example.com/library", ModuleDir: ".", Main: true,
			PackagesCount: 1, RetainedPackagesCount: 1,
			Coverage: gofacts.ModuleCoverage{
				State: gofacts.CoverageComplete, PackagesDiscovered: 1, PackagesRetained: 1,
			},
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/library", Name: "library", ModuleID: "module-root",
			ModulePath: "example.com/library", PackageDir: ".", ModuleRelativeDir: ".", Locality: "local",
			DeclarationsScanned: true, LoadCompleteness: completeGoPackageLoad(),
			Declarations: []gofacts.PackageDeclaration{{
				Kind: gofacts.PackageDeclarationFunc, Name: "NewBot",
				Path: "api.go", Line: 1, Column: 6,
			}},
		}},
		PackagesCount: 1, RetainedPackagesCount: 1,
		Coverage: gofacts.Coverage{
			State: gofacts.CoverageComplete, ModulesDiscovered: 1, ModulesAvailable: 1,
			PackagesDiscovered: 1, PackagesRetained: 1,
		},
	}, analysistarget.Options{Override: "example.com/library"})
	if err != nil || resolution.Selected == nil {
		t.Fatalf("resolve library: resolution=%#v err=%v", resolution, err)
	}
	if resolution.Selected.Kind != analysistarget.KindModuleLibrary ||
		len(resolution.Selected.LibraryPackages) != 1 {
		t.Fatalf("resolve module library: resolution=%#v", resolution)
	}
	target := resolution.Selected.Snapshot()
	input := atlasstudy.Input{
		Atlas: repositoryatlas.Atlas{
			Version: repositoryatlas.Version,
			Units: []repositoryatlas.Unit{
				{ID: "repo", Kind: repositoryatlas.UnitRepository, Name: "repository"},
				{ID: "module-root", Kind: repositoryatlas.UnitModule, ParentID: "repo", Name: "example.com/library"},
				{ID: "unit-package-canonical", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "example.com/library"},
			},
		},
		Architecture: atlasstudy.ArchitectureInput{Source: "local_packages", Title: "Library"},
		Language:     atlasstudy.LanguageEnglish, Limits: atlasstudy.DefaultLimits(),
		AnalysisTargetRoot: &atlasstudy.AnalysisTargetRootScope{
			AnalysisTarget: target,
			Packages: []atlasstudy.AnalysisTargetRootPackage{{
				Package: target.LibraryPackages[0], UnitID: "unit-package-canonical",
			}},
		},
	}
	type item struct {
		kind  atlasstudy.ReadingTargetKind
		label string
		path  string
		line  int
	}
	items := []item{
		{atlasstudy.ReadingTargetFunction, "Newline", "api.go", 1},
		{atlasstudy.ReadingTargetFunction, "Open", "api.go", 2},
		{atlasstudy.ReadingTargetFunction, "NewBot", "constructors.go", 50},
	}
	for index := len(items); index < count; index++ {
		receiver := "Bot"
		if index%2 == 1 {
			receiver = "File"
		}
		items = append(items, item{
			atlasstudy.ReadingTargetMethod, fmt.Sprintf("%s.Method%02d", receiver, index),
			"methods.go", index + 10,
		})
	}
	for index, value := range items {
		id := fmt.Sprintf("root-%03d", count-index)
		supportID := "support-" + id
		input.ReadingTargets = append(input.ReadingTargets, atlasstudy.ReadingTarget{
			ID: id, PrincipalRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefUnit, ID: "unit-package-canonical"}},
			Kind: value.kind, Label: value.label, Symbol: value.label,
			Fact: "Exact exported callable declaration.", Authority: repositoryatlas.AuthorityResolved,
			Location: evidence.Location{Path: value.path, Line: value.line, Column: 6},
		})
		input.ReadingSupports = append(input.ReadingSupports, atlasstudy.ReadingSupport{
			ID: supportID, TargetID: id, PackageBucket: "unit-package-canonical",
			Role: atlasstudy.SupportAnalysisTargetRoot, Authority: repositoryatlas.AuthorityResolved,
		})
		input.RouteSpans = append(input.RouteSpans, atlasstudy.RouteSpan{
			ID: "span-" + id, Kind: atlasstudy.RouteSpanFocused,
			QuestionEnglish: "How does this callable participate in the selected library's public API?",
			QuestionRussian: "Как этот вызываемый объект участвует в публичном API выбранной библиотеки?",
			TargetJob:       atlasstudy.JobIntegrate, LearningStage: atlasstudy.StageIntegration,
			RequiredSupportIDs: []string{supportID}, AllowedTargetIDs: []string{id},
		})
	}
	return input
}
