package atlasstudy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestD280ModuleLibraryFrontierKeepsPackageBreadthWithin32AndIsPermutationStable(t *testing.T) {
	input := d280ModuleLibraryInput(t, 36, 4)
	product, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	coverage := product.Coverage()
	if coverage.TargetsConsidered != 40 || coverage.TargetsSelected != 32 ||
		coverage.SpansConsidered != 40 || coverage.SpansSelected != 32 || coverage.Complete {
		t.Fatalf("coverage = %#v", coverage)
	}
	selectedByPackage := make(map[string]int)
	for _, count := range coverage.PerPackage {
		selectedByPackage[count.Key] = count.Selected
	}
	if selectedByPackage["unit-api"] == 0 || selectedByPackage["unit-storage"] == 0 ||
		selectedByPackage["unit-api"]+selectedByPackage["unit-storage"] != 32 {
		t.Fatalf("module package breadth = %#v", coverage.PerPackage)
	}

	selected, err := SelectAnalysisTargetRootFrontier(input)
	if err != nil {
		t.Fatalf("SelectAnalysisTargetRootFrontier: %v", err)
	}
	ordered, err := OrderAnalysisTargetRootReadingTargets(selected)
	if err != nil {
		t.Fatalf("OrderAnalysisTargetRootReadingTargets: %v", err)
	}
	if len(ordered) != 32 {
		t.Fatalf("ordered roots = %d, want 32", len(ordered))
	}
	selectedUnits := make(map[string]struct{})
	for _, target := range ordered {
		if len(target.PrincipalRefs) != 1 || target.PrincipalRefs[0].Kind != RefUnit {
			t.Fatalf("selected root has invalid principal: %#v", target)
		}
		selectedUnits[target.PrincipalRefs[0].ID] = struct{}{}
	}
	if _, ok := selectedUnits["unit-api"]; !ok {
		t.Fatal("32-root frontier omitted the first public package")
	}
	if _, ok := selectedUnits["unit-storage"]; !ok {
		t.Fatal("32-root frontier omitted the second public package")
	}

	request, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("RequestRecord: %v", err)
	}
	if request.Version != 9 || request.PromptVersion != "atlas-study-prompt-v15" ||
		request.AnalysisTargetRoot == nil || request.AnalysisTargetRoot.UnitID != "" ||
		request.AnalysisTargetRoot.AnalysisTarget.Kind != analysistarget.KindModuleLibrary ||
		len(request.AnalysisTargetRoot.Packages) != 2 {
		t.Fatalf("fresh v9 module-library request scope = %#v", request.AnalysisTargetRoot)
	}

	permuted := cloneTestInput(input)
	slices.Reverse(permuted.Atlas.Units)
	slices.Reverse(permuted.ReadingTargets)
	slices.Reverse(permuted.ReadingSupports)
	slices.Reverse(permuted.RouteSpans)
	permutedProduct, err := Compile(permuted)
	if err != nil {
		t.Fatalf("Compile permuted: %v", err)
	}
	if !reflect.DeepEqual(product.WireJSON(), permutedProduct.WireJSON()) ||
		product.CatalogSHA256() != permutedProduct.CatalogSHA256() ||
		!reflect.DeepEqual(product.Coverage(), permutedProduct.Coverage()) {
		t.Fatal("module-library frontier changed under producer permutation")
	}
}

func TestD280ModuleLibraryRejectsArbitrarySwappedAndMissingPackageUnits(t *testing.T) {
	base := d280ModuleLibraryInput(t, 2, 2)

	t.Run("swapped bindings", func(t *testing.T) {
		input := cloneTestInput(base)
		input.AnalysisTargetRoot.Packages[0].UnitID, input.AnalysisTargetRoot.Packages[1].UnitID =
			input.AnalysisTargetRoot.Packages[1].UnitID, input.AnalysisTargetRoot.Packages[0].UnitID
		if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "package Unit binding mismatch") {
			t.Fatalf("Compile swapped bindings error = %v", err)
		}
	})

	t.Run("missing binding", func(t *testing.T) {
		input := cloneTestInput(base)
		input.AnalysisTargetRoot.Packages = input.AnalysisTargetRoot.Packages[:1]
		if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "invalid selected AnalysisTarget root scope") {
			t.Fatalf("Compile missing binding error = %v", err)
		}
	})

	t.Run("missing Atlas Unit", func(t *testing.T) {
		input := cloneTestInput(base)
		input.Atlas.Units = slices.DeleteFunc(input.Atlas.Units, func(unit repositoryatlas.Unit) bool {
			return unit.ID == "unit-storage"
		})
		if _, err := Compile(input); err == nil {
			t.Fatal("Compile accepted a root whose exact Atlas package Unit is missing")
		}
	})

	t.Run("arbitrary Unit", func(t *testing.T) {
		input := cloneTestInput(base)
		input.Atlas.Units = append(input.Atlas.Units, repositoryatlas.Unit{
			ID: "unit-arbitrary", Kind: repositoryatlas.UnitPackage,
			ParentID: "module-root", Name: "example.com/library/other",
		})
		input.AnalysisTargetRoot.Packages[0].UnitID = "unit-arbitrary"
		for index := range input.ReadingTargets {
			if input.ReadingTargets[index].PrincipalRefs[0].ID == "unit-api" {
				input.ReadingTargets[index].PrincipalRefs[0].ID = "unit-arbitrary"
			}
		}
		for index := range input.ReadingSupports {
			if input.ReadingSupports[index].PackageBucket == "unit-api" {
				input.ReadingSupports[index].PackageBucket = "unit-arbitrary"
			}
		}
		if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "package Unit binding mismatch") {
			t.Fatalf("Compile arbitrary Unit error = %v", err)
		}
	})

	t.Run("cross-package bucket substitution", func(t *testing.T) {
		input := cloneTestInput(base)
		for index := range input.ReadingSupports {
			if input.ReadingSupports[index].PackageBucket == "unit-api" {
				input.ReadingSupports[index].PackageBucket = "unit-storage"
				break
			}
		}
		if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "exact package Unit") {
			t.Fatalf("Compile cross-package bucket error = %v", err)
		}
	})
}

func TestD280CrossPackageRootsResolveAndRejectSwappedResponsePrincipal(t *testing.T) {
	input := d280ModuleLibraryInput(t, 2, 2)
	product, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MockResponse(request)
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := product.ResolveResponseJSON(raw)
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	principalUnits := make(map[string]struct{})
	for _, direction := range result.Directions {
		for _, principal := range direction.PrincipalRefs {
			if principal.Kind == RefUnit {
				principalUnits[principal.ID] = struct{}{}
			}
		}
	}
	if len(principalUnits) != 2 {
		t.Fatalf("resolved cross-package principals = %#v", principalUnits)
	}
	if _, _, _, err := ReplayResponseRecord(request, raw); err != nil {
		t.Fatalf("ReplayResponseRecord: %v", err)
	}

	var response responseEnvelope
	if err := decodeStrict(raw, &response); err != nil {
		t.Fatal(err)
	}
	for index, rawDirection := range response.Directions {
		var direction providerDirection
		if err := decodeStrict(rawDirection, &direction); err != nil {
			t.Fatal(err)
		}
		target := product.byRef[direction.Reading[0].TargetRef]
		if len(target.PrincipalRefs) != 1 {
			t.Fatalf("fixture target principals = %#v", target.PrincipalRefs)
		}
		wrongUnitID := "unit-api"
		if target.PrincipalRefs[0].ID == wrongUnitID {
			wrongUnitID = "unit-storage"
		}
		wrongUnit := product.byCanonical[CanonicalRef{Kind: RefUnit, ID: wrongUnitID}]
		direction.PrincipalRefs = []string{wrongUnit.Ref}
		encoded, err := json.Marshal(direction)
		if err != nil {
			t.Fatal(err)
		}
		response.Directions[index] = encoded
	}
	tampered, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := product.ResolveResponseJSON(tampered); err == nil ||
		!strings.Contains(err.Error(), "no valid Study directions") {
		t.Fatalf("swapped response principal error = %v", err)
	}
}

func TestD280ReplayRejectsScopeTamperAndKeepsEveryPackageUnitPrivate(t *testing.T) {
	product, err := Compile(d280ModuleLibraryInput(t, 2, 2))
	if err != nil {
		t.Fatal(err)
	}
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MockResponse(request)
	if err != nil {
		t.Fatal(err)
	}

	tamperedRequest := request
	tamperedRequest.AnalysisTargetRoot = cloneAnalysisTargetRootScope(request.AnalysisTargetRoot)
	tamperedRequest.AnalysisTargetRoot.Packages[0].UnitID,
		tamperedRequest.AnalysisTargetRoot.Packages[1].UnitID =
		tamperedRequest.AnalysisTargetRoot.Packages[1].UnitID,
		tamperedRequest.AnalysisTargetRoot.Packages[0].UnitID
	d277RebindRequestCatalog(t, &tamperedRequest)
	if _, _, _, err := ReplayResponseRecord(tamperedRequest, raw); err == nil ||
		!strings.Contains(err.Error(), "package Unit binding mismatch") {
		t.Fatalf("replay swapped scope error = %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	brief := response["brief"].(map[string]any)
	whatItIs := brief["what_it_is"].(map[string]any)
	whatItIs["text"] = request.AnalysisTargetRoot.Packages[1].UnitID
	identityEcho, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := product.ResolveResponseJSON(identityEcho); err == nil ||
		!strings.Contains(err.Error(), "canonical identity") {
		t.Fatalf("live package Unit echo error = %v", err)
	}
	if _, _, _, err := ReplayResponseRecord(request, identityEcho); err == nil ||
		!strings.Contains(err.Error(), "canonical identity") {
		t.Fatalf("replay package Unit echo error = %v", err)
	}
}

func TestD280LegacyPackageLibraryRemainsLiveOnlyAndCannotCreateV9Artifact(t *testing.T) {
	input := d277LibraryInput(t, 4)
	target := analysistarget.Target{
		Version: analysistarget.Version, Kind: analysistarget.KindLibraryPackage,
		ModuleID: "module-root", ModulePath: "example.com/library", ModuleDir: ".",
		PackagePath: "example.com/library", PackageDir: ".",
		RootBoundary: analysistarget.RootBoundaryExactPublicAPI,
		Roots:        []analysistarget.Root{},
	}
	target.Ref = d280SealLegacyTarget(t, target)
	if err := target.Validate(); err != nil {
		t.Fatalf("legacy target fixture: %v", err)
	}
	input.AnalysisTargetRoot = &AnalysisTargetRootScope{
		AnalysisTarget: target, UnitID: "unit-package",
	}
	product, err := Compile(input)
	if err != nil {
		t.Fatalf("live legacy source compatibility: %v", err)
	}
	if _, err := product.RequestRecord(); err == nil ||
		!strings.Contains(err.Error(), "module library") {
		t.Fatalf("legacy target created a fresh v9 artifact: %v", err)
	}
}

func d280ModuleLibraryInput(t *testing.T, apiRoots, storageRoots int) Input {
	t.Helper()
	if apiRoots <= 0 || storageRoots <= 0 {
		t.Fatal("D280 fixture requires roots in both packages")
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: "example.com/library", ModuleDir: ".", Main: true,
			PackagesCount: 2, RetainedPackagesCount: 2,
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: "example.com/library/api", Name: "api", ModuleID: "module-root",
				ModulePath: "example.com/library", PackageDir: "api", ModuleRelativeDir: "api",
				Locality: "local", DeclarationsScanned: true, LoadCompleteness: completeAtlasPackageLoad(),
				Declarations: []gofacts.PackageDeclaration{{
					Kind: gofacts.PackageDeclarationType, Name: "Client",
					Path: "api/api.go", Line: 1, Column: 6,
				}},
			},
			{
				CanonicalPath: "example.com/library/storage", Name: "storage", ModuleID: "module-root",
				ModulePath: "example.com/library", PackageDir: "storage", ModuleRelativeDir: "storage",
				Locality: "local", DeclarationsScanned: true, LoadCompleteness: completeAtlasPackageLoad(),
				Declarations: []gofacts.PackageDeclaration{{
					Kind: gofacts.PackageDeclarationType, Name: "Store",
					Path: "storage/storage.go", Line: 1, Column: 6,
				}},
			},
		},
	}
	resolution, err := analysistarget.Resolve(facts, analysistarget.Options{})
	if err != nil || resolution.Selected == nil || resolution.Selected.Kind != analysistarget.KindModuleLibrary {
		t.Fatalf("resolve module library: resolution=%#v err=%v", resolution, err)
	}
	target := resolution.Selected.Snapshot()
	unitByPackage := map[string]string{
		"example.com/library/api":     "unit-api",
		"example.com/library/storage": "unit-storage",
	}
	input := Input{
		Atlas: repositoryatlas.Atlas{
			Version: repositoryatlas.Version,
			Units: []repositoryatlas.Unit{
				{ID: "unit-repository", Kind: repositoryatlas.UnitRepository, Name: "repository"},
				{ID: "module-root", Kind: repositoryatlas.UnitModule, ParentID: "unit-repository", Name: "example.com/library"},
				{ID: "unit-api", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "example.com/library/api"},
				{ID: "unit-storage", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "example.com/library/storage"},
			},
		},
		Architecture: ArchitectureInput{Source: "local_packages", Title: "Library"},
		Language:     LanguageEnglish,
		Limits:       DefaultLimits(),
		AnalysisTargetRoot: &AnalysisTargetRootScope{
			AnalysisTarget: target,
		},
	}
	for _, pkg := range target.LibraryPackages {
		input.AnalysisTargetRoot.Packages = append(
			input.AnalysisTargetRoot.Packages,
			AnalysisTargetRootPackage{Package: pkg, UnitID: unitByPackage[pkg.PackagePath]},
		)
	}
	addRoots := func(packageName, packageDir, unitID, idPrefix string, count int) {
		for index := range count {
			kind := ReadingTargetMethod
			label := fmt.Sprintf("Client.Method%02d", index)
			if index == 0 {
				kind = ReadingTargetFunction
				label = "New" + packageName
			}
			id := fmt.Sprintf("root-%s-%03d", idPrefix, count-index)
			supportID := "support-" + id
			input.ReadingTargets = append(input.ReadingTargets, ReadingTarget{
				ID: id, PrincipalRefs: []CanonicalRef{{Kind: RefUnit, ID: unitID}},
				Kind: kind, Label: label, Symbol: label,
				Fact:      "Exact exported callable declaration in one selected module public package.",
				Authority: repositoryatlas.AuthorityResolved,
				Location: evidence.Location{
					Path: packageDir + "/api.go", Line: index + 10, Column: 6,
				},
			})
			input.ReadingSupports = append(input.ReadingSupports, ReadingSupport{
				ID: supportID, TargetID: id, PackageBucket: unitID,
				Role: SupportAnalysisTargetRoot, Authority: repositoryatlas.AuthorityResolved,
			})
			input.RouteSpans = append(input.RouteSpans, RouteSpan{
				ID: "span-" + id, Kind: RouteSpanFocused,
				QuestionEnglish: "How does this callable participate in the selected module library API?",
				QuestionRussian: "Как этот вызываемый объект участвует в API выбранной библиотеки модуля?",
				TargetJob:       JobIntegrate, LearningStage: StageIntegration,
				RequiredSupportIDs: []string{supportID}, AllowedTargetIDs: []string{id},
			})
		}
	}
	// Deliberately opposing opaque ID prefixes prove that source/package
	// authority, not producer slice or hash-like ID order, owns the frontier.
	addRoots("API", "api", "unit-api", "z-api", apiRoots)
	addRoots("Storage", "storage", "unit-storage", "a-storage", storageRoots)
	return input
}

func d280SealLegacyTarget(t *testing.T, target analysistarget.Target) string {
	t.Helper()
	target.Ref = ""
	encoded, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	return "at-" + hex.EncodeToString(sum[:12])
}
