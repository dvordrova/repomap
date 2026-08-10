package atlasstudy

import (
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

func TestD277LibraryPublicAPIFrontierIsUsefulBoundedAndPermutationStable(t *testing.T) {
	input := d277LibraryInput(t, 40)
	product, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if product.Coverage().TargetsConsidered != 40 || product.Coverage().TargetsSelected != 32 ||
		product.Coverage().Complete {
		t.Fatalf("coverage = %#v", product.Coverage())
	}
	selected, err := SelectAnalysisTargetRootFrontier(input)
	if err != nil {
		t.Fatalf("SelectAnalysisTargetRootFrontier: %v", err)
	}
	ordered, err := OrderAnalysisTargetRootReadingTargets(selected)
	if err != nil {
		t.Fatalf("OrderAnalysisTargetRootReadingTargets: %v", err)
	}
	if len(ordered) != 32 || ordered[0].Label != "NewBot" || ordered[1].Label != "Newline" ||
		ordered[2].Label != "Open" {
		t.Fatalf("public API order starts %#v", ordered[:min(4, len(ordered))])
	}
	seenFamilies := map[string]bool{}
	for _, target := range ordered {
		if target.Kind != ReadingTargetMethod {
			continue
		}
		receiver, _, _ := strings.Cut(target.Label, ".")
		seenFamilies[receiver] = true
	}
	if !seenFamilies["Bot"] || !seenFamilies["File"] {
		t.Fatalf("receiver-family round robin lost breadth: %v", seenFamilies)
	}
	if wire := string(product.WireJSON()); strings.Contains(wire, "analysis_target_root") ||
		strings.Contains(wire, input.AnalysisTargetRoot.AnalysisTarget.Ref) {
		t.Fatalf("private selected target scope leaked to provider wire: %s", wire)
	}

	permuted := input
	permuted.ReadingTargets = append([]ReadingTarget(nil), input.ReadingTargets...)
	permuted.ReadingSupports = append([]ReadingSupport(nil), input.ReadingSupports...)
	permuted.RouteSpans = cloneRouteSpans(input.RouteSpans)
	slices.Reverse(permuted.ReadingTargets)
	slices.Reverse(permuted.ReadingSupports)
	slices.Reverse(permuted.RouteSpans)
	permutedProduct, err := Compile(permuted)
	if err != nil {
		t.Fatalf("Compile permuted: %v", err)
	}
	if !reflect.DeepEqual(product.WireJSON(), permutedProduct.WireJSON()) ||
		product.CatalogSHA256() != permutedProduct.CatalogSHA256() {
		t.Fatal("public API frontier changed under candidate permutation")
	}
}

func TestD277LibraryPublicAPIRootRejectsArbitraryPackageUnit(t *testing.T) {
	input := d277LibraryInput(t, 4)
	input.Atlas.Units = append(input.Atlas.Units, repositoryatlas.Unit{
		ID: "unit-package-other", Kind: repositoryatlas.UnitPackage,
		ParentID: "module-root", Name: "example.com/library/other",
	})
	for index := range input.ReadingTargets {
		input.ReadingTargets[index].PrincipalRefs = []CanonicalRef{{
			Kind: RefUnit, ID: "unit-package-other",
		}}
	}
	for index := range input.ReadingSupports {
		input.ReadingSupports[index].PackageBucket = "unit-package-other"
	}
	if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "exact package Unit") {
		t.Fatalf("Compile arbitrary Unit error = %v", err)
	}
}

func TestD277RequestArtifactBindsSelectedPackageUnitAndCallableKind(t *testing.T) {
	input := d277LibraryInput(t, 4)
	input.Atlas.Units = append(input.Atlas.Units, repositoryatlas.Unit{
		ID: "unit-package-other", Kind: repositoryatlas.UnitPackage,
		ParentID: "module-root", Name: "example.com/library/other",
	})
	product, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}

	wrongKind := request
	wrongKind.Catalog = cloneCatalog(request.Catalog)
	for index := range wrongKind.Catalog {
		if wrongKind.Catalog[index].Kind == RefReadingTarget {
			wrongKind.Catalog[index].ReadingTargetKind = ReadingTargetType
			break
		}
	}
	d277RebindRequestCatalog(t, &wrongKind)
	if _, err := EncodeRequestRecord(wrongKind); err == nil ||
		!strings.Contains(err.Error(), "public API root") {
		t.Fatalf("non-callable root artifact error = %v", err)
	}

	wrongUnit := request
	wrongUnit.Catalog = cloneCatalog(request.Catalog)
	wrongUnit.AnalysisTargetRoot = cloneAnalysisTargetRootScope(request.AnalysisTargetRoot)
	wrongUnit.AnalysisTargetRoot.UnitID = "unit-package-other"
	for index := range wrongUnit.Catalog {
		switch wrongUnit.Catalog[index].Kind {
		case RefReadingTarget:
			wrongUnit.Catalog[index].PrincipalRefs = []CanonicalRef{{Kind: RefUnit, ID: "unit-package-other"}}
		case RefRouteSupport:
			if wrongUnit.Catalog[index].SupportRole == SupportAnalysisTargetRoot {
				wrongUnit.Catalog[index].PackageBucket = "unit-package-other"
			}
		}
	}
	d277RebindRequestCatalog(t, &wrongUnit)
	if _, err := EncodeRequestRecord(wrongUnit); err == nil ||
		!strings.Contains(err.Error(), "exact package Unit") {
		t.Fatalf("arbitrary selected Unit artifact error = %v", err)
	}
}

func TestD277SelectedTargetIdentityIsPrivateInLiveAndReplayValidation(t *testing.T) {
	product, err := Compile(d277LibraryInput(t, 4))
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
	var response map[string]any
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	brief := response["brief"].(map[string]any)
	whatItIs := brief["what_it_is"].(map[string]any)
	whatItIs["text"] = request.AnalysisTargetRoot.AnalysisTarget.Ref
	tampered, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := product.ResolveResponseJSON(tampered); err == nil ||
		!strings.Contains(err.Error(), "canonical identity") {
		t.Fatalf("live selected-target identity echo error = %v", err)
	}
	if _, _, _, err := ReplayResponseRecord(request, tampered); err == nil ||
		!strings.Contains(err.Error(), "canonical identity") {
		t.Fatalf("replay selected-target identity echo error = %v", err)
	}
}

func d277RebindRequestCatalog(t *testing.T, request *RequestRecord) {
	t.Helper()
	material := catalogMaterial{
		Version: Version, AtlasSHA256: request.AtlasSHA256,
		ArchitectureSHA256: request.ArchitectureSHA256, Language: request.Language,
		Limits: DefaultLimits(), ProjectionSHA256: request.WireSHA256,
		Coverage:           cloneCandidateCoverage(request.CandidateCoverage),
		AnalysisTargetRoot: cloneAnalysisTargetRootScope(request.AnalysisTargetRoot),
		Objects:            request.Catalog,
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	request.CatalogSHA256 = digest(encoded)
	request.CatalogRef = fmt.Sprintf("atlas-study-v%d-%s", Version, request.CatalogSHA256)
}

func d277LibraryInput(t *testing.T, count int) Input {
	t.Helper()
	resolution, err := analysistarget.Resolve(gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: "example.com/library", ModuleDir: ".", Main: true,
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/library", Name: "library", ModuleID: "module-root",
			ModulePath: "example.com/library", PackageDir: ".", ModuleRelativeDir: ".",
			Locality: "local",
		}},
	}, analysistarget.Options{})
	if err != nil || resolution.Selected == nil {
		t.Fatalf("resolve library target: resolution=%#v err=%v", resolution, err)
	}
	target := resolution.Selected.Snapshot()
	input := Input{
		Atlas: repositoryatlas.Atlas{
			Version: repositoryatlas.Version,
			Units: []repositoryatlas.Unit{
				{ID: "unit-repository", Kind: repositoryatlas.UnitRepository, Name: "repository"},
				{ID: "module-root", Kind: repositoryatlas.UnitModule, ParentID: "unit-repository", Name: "example.com/library"},
				{ID: "unit-package", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "example.com/library"},
			},
		},
		Architecture: ArchitectureInput{Source: "local_packages", Title: "Library"},
		Language:     LanguageEnglish,
		Limits:       DefaultLimits(),
		AnalysisTargetRoot: &AnalysisTargetRootScope{
			AnalysisTarget: target, UnitID: "unit-package",
		},
	}
	labels := []struct {
		kind     ReadingTargetKind
		label    string
		path     string
		line     int
		column   int
		receiver string
	}{
		{kind: ReadingTargetFunction, label: "Newline", path: "api.go", line: 1, column: 6},
		{kind: ReadingTargetFunction, label: "Open", path: "api.go", line: 2, column: 6},
		{kind: ReadingTargetFunction, label: "NewBot", path: "constructors.go", line: 50, column: 6},
	}
	for index := len(labels); index < count; index++ {
		receiver := "Bot"
		if index%2 == 1 {
			receiver = "File"
		}
		labels = append(labels, struct {
			kind     ReadingTargetKind
			label    string
			path     string
			line     int
			column   int
			receiver string
		}{
			kind: ReadingTargetMethod, label: fmt.Sprintf("%s.Method%02d", receiver, index),
			path: "methods.go", line: index + 10, column: 6, receiver: receiver,
		})
	}
	for index, item := range labels {
		id := fmt.Sprintf("root-%03d-%s", count-index, strings.ReplaceAll(item.label, ".", "-"))
		supportID := "support-" + id
		input.ReadingTargets = append(input.ReadingTargets, ReadingTarget{
			ID: id, PrincipalRefs: []CanonicalRef{{Kind: RefUnit, ID: "unit-package"}},
			Kind: item.kind, Label: item.label, Symbol: item.label,
			Fact:      "Exact exported callable declaration in the selected library package.",
			Authority: repositoryatlas.AuthorityResolved,
			Location:  evidence.Location{Path: item.path, Line: item.line, Column: item.column},
		})
		input.ReadingSupports = append(input.ReadingSupports, ReadingSupport{
			ID: supportID, TargetID: id, PackageBucket: "unit-package",
			Role: SupportAnalysisTargetRoot, Authority: repositoryatlas.AuthorityResolved,
		})
		input.RouteSpans = append(input.RouteSpans, RouteSpan{
			ID: "span-" + id, Kind: RouteSpanFocused,
			QuestionEnglish: "How does this callable participate in the selected library's public API?",
			QuestionRussian: "Как этот вызываемый объект участвует в публичном API выбранной библиотеки?",
			TargetJob:       JobIntegrate, LearningStage: StageIntegration,
			RequiredSupportIDs: []string{supportID}, AllowedTargetIDs: []string{id},
		})
	}
	return input
}
