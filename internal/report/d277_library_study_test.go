package report

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestD277TelebotLibraryStudyUsesExactCallablePublicAPIWithoutComponents(t *testing.T) {
	data := d277LibraryReportData(t, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "Assembly", Path: "assembly.go", Line: 3, Column: 6},
		{Kind: gofacts.PackageDeclarationFunc, Name: "File", Path: "bot.go", Line: 20, Column: 6, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationFunc, Name: "NewBot", Path: "bot.go", Line: 5, Column: 6, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Bot", Name: "Raw", Path: "bot.go", Line: 12, Column: 15, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "hidden", Name: "Download", Path: "download.go", Line: 7, Column: 19, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationType, Name: "Bot", Path: "bot.go", Line: 3, Column: 6},
	})
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}
	if input.AnalysisTargetRoot == nil ||
		input.AnalysisTargetRoot.AnalysisTarget.Ref != data.AnalysisTarget.Ref ||
		len(input.Architecture.Components) != 0 || len(input.Surfaces) != 0 {
		t.Fatalf("library root scope/input = %#v", input)
	}
	labels := make([]string, 0, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		labels = append(labels, target.Label)
		if target.Owner != (atlasstudy.CanonicalRef{}) || len(target.RelatedComponentIDs) != 0 ||
			len(target.PrincipalRefs) != 1 || target.PrincipalRefs[0].Kind != atlasstudy.RefUnit {
			t.Fatalf("public API target inferred a component owner: %#v", target)
		}
	}
	slices.Sort(labels)
	if want := []string{"Bot.Raw", "File", "NewBot", "hidden.Download"}; !slices.Equal(labels, want) {
		t.Fatalf("library readings = %v, want %v", labels, want)
	}
	if _, err := atlasstudy.Compile(input); err != nil {
		t.Fatalf("compile zero-surface library Study: %v", err)
	}
	collectOpenablePaths(data)
	for _, want := range []string{"bot.go", "download.go"} {
		if !slices.Contains(data.OpenablePaths, want) {
			t.Fatalf("callable root path %q not authorized: %v", want, data.OpenablePaths)
		}
	}
	if slices.Contains(data.OpenablePaths, "assembly.go") {
		t.Fatalf("bodyless declaration became a code-oriented Study root: %v", data.OpenablePaths)
	}
}

func TestD277LibraryStudyRejectsDeclarationMovedWithFilesOutsideSelectedPackage(t *testing.T) {
	data := d277LibraryReportData(t, []gofacts.PackageDeclaration{{
		Kind: gofacts.PackageDeclarationFunc, Name: "NewClient",
		Path: "other/client.go", Line: 3, Column: 6, ExecutableBody: true,
	}})
	data.repositoryGoFacts.Packages[0].Files = []string{"other/client.go"}
	if _, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish); err == nil ||
		!strings.Contains(err.Error(), "outside its build-selected package files") {
		t.Fatalf("moved declaration error = %v", err)
	}
}

func TestD277TypesAndBodylessOnlyLibraryIsTypedInsufficientCatalog(t *testing.T) {
	data := d277LibraryReportData(t, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "Assembly", Path: "assembly.go", Line: 3, Column: 6},
		{Kind: gofacts.PackageDeclarationType, Name: "Client", Path: "client.go", Line: 2, Column: 6},
	})
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.ReadingTargets) != 0 || input.AnalysisTargetRoot == nil {
		t.Fatalf("types-only input = %#v", input)
	}
	_, compileErr := atlasstudy.Compile(input)
	var unavailable *atlasstudy.CandidateUnavailableError
	if !errors.As(compileErr, &unavailable) ||
		!strings.Contains(compileErr.Error(), "no observed support roles") {
		t.Fatalf("types-only compile error = %T %v", compileErr, compileErr)
	}
	if AtlasStudyInputHasMinimumCatalog(input) {
		t.Fatal("types-only library unexpectedly has a minimum Study catalog")
	}
}

func TestD277TypesOnlySavedReportRehydratesPrivateFactsBeforeUncalledProjection(t *testing.T) {
	data := d277LibraryReportData(t, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "Assembly", Path: "assembly.go", Line: 3, Column: 6},
		{Kind: gofacts.PackageDeclarationType, Name: "Client", Path: "client.go", Line: 2, Column: 6},
	})
	runDir := t.TempDir()
	snapshot, err := json.Marshal(map[string]any{"go_facts": *data.repositoryGoFacts})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", string(snapshot))
	persisted := *data
	persisted.repositoryGoFacts = nil
	persisted.ArchitectureCanvas = &ArchitectureCanvas{}
	status, studyMap, err := readAtlasStudyReportProduct(runDir, &persisted)
	if err != nil {
		t.Fatalf("read types-only saved report: %v", err)
	}
	if studyMap != nil || status == nil || status.State != atlasstudy.ProductStateUnavailable ||
		status.UnavailableCode != AtlasStudyUnavailableInsufficientCatalog ||
		status.ProjectionVersion != AtlasStudyReportProjectionVersion {
		t.Fatalf("types-only saved projection = %#v / %#v", status, studyMap)
	}
}

func TestD277SavedReportCannotRepairMissingPublicRootSourceAuthority(t *testing.T) {
	data := d277LibraryReportData(t, []gofacts.PackageDeclaration{{
		Kind: gofacts.PackageDeclarationFunc, Name: "NewBot",
		Path: "bot.go", Line: 5, Column: 6, ExecutableBody: true,
	}})
	collectOpenablePaths(data)
	if !slices.Contains(data.OpenablePaths, "bot.go") {
		t.Fatalf("live report did not capture public root path: %v", data.OpenablePaths)
	}
	runDir := t.TempDir()
	snapshot, err := json.Marshal(map[string]any{"go_facts": *data.repositoryGoFacts})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", string(snapshot))
	persisted := *data
	persisted.repositoryGoFacts = nil
	persisted.ArchitectureCanvas = &ArchitectureCanvas{}
	if _, _, err := readPersistedAtlasStudyReportProduct(runDir, &persisted); err != nil {
		t.Fatalf("exact saved source authority: %v", err)
	}
	persisted.OpenablePaths = nil
	if _, _, err := readPersistedAtlasStudyReportProduct(runDir, &persisted); err == nil ||
		!strings.Contains(err.Error(), "saved source authority omits public API root") {
		t.Fatalf("missing saved public root authority error = %v", err)
	}
}

func TestD277ReadRunDirDerivesMultifilePublicRootSourceAuthority(t *testing.T) {
	data := d277LibraryReportData(t, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "NewBot", Path: "bot.go", Line: 5, Column: 6, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Bot", Name: "Download", Path: "download.go", Line: 7, Column: 15, ExecutableBody: true},
	})
	runDir := t.TempDir()
	snapshot, err := json.Marshal(map[string]any{
		"repo_name":       "repository",
		"analysis_target": data.AnalysisTarget,
		"go_facts":        *data.repositoryGoFacts,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", string(snapshot))
	atlasJSON, err := repositoryatlas.CanonicalJSON(*data.RepositoryAtlas)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, repositoryatlas.ArtifactFilename), atlasJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	replayed, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir: %v", err)
	}
	for _, want := range []string{"bot.go", "download.go"} {
		if !slices.Contains(replayed.OpenablePaths, want) {
			t.Fatalf("reconstructed source authority omits %q: %v", want, replayed.OpenablePaths)
		}
	}
}

func TestD277SavedScoutRequiresCompleteSelectedRootAccounting(t *testing.T) {
	data := d277LibraryReportData(t, []gofacts.PackageDeclaration{
		{Kind: gofacts.PackageDeclarationFunc, Name: "File", Path: "bot.go", Line: 20, Column: 6, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationFunc, Name: "NewBot", Path: "bot.go", Line: 5, Column: 6, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "Bot", Name: "Raw", Path: "bot.go", Line: 12, Column: 15, ExecutableBody: true},
		{Kind: gofacts.PackageDeclarationMethod, Receiver: "File", Name: "Download", Path: "download.go", Line: 7, Column: 19, ExecutableBody: true},
	})
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatal(err)
	}
	input, err = atlasstudy.SelectAnalysisTargetRootFrontier(input)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := atlasstudy.OrderAnalysisTargetRootReadingTargets(input)
	if err != nil {
		t.Fatal(err)
	}
	spanByTarget := make(map[string]string, len(input.RouteSpans))
	for _, span := range input.RouteSpans {
		if len(span.AllowedTargetIDs) == 1 {
			spanByTarget[span.AllowedTargetIDs[0]] = span.ID
		}
	}
	pack := func(ordinal int) themestudy.SeedPack {
		target := ordered[ordinal-1]
		return themestudy.SeedPack{Seed: themestudy.SeedSpec{
			Ref: "a" + strconv.Itoa(ordinal), Path: target.Location.Path,
			Line: target.Location.Line, Symbol: target.Symbol,
			CanonicalSpanID: spanByTarget[target.ID],
			Kind:            "focused", Role: themestudy.RolePublicAPI,
			Provenance: "d211_span_reading_target",
		}}
	}
	request := themestudy.ScoutRequest{
		SeedPacks: themestudy.SeedPackResult{Packs: []themestudy.SeedPack{pack(1)}},
	}
	if err := validateThemeScoutRequestAgainstInput(request, input); err == nil ||
		!strings.Contains(err.Error(), "do not account") {
		t.Fatalf("silently shrunken Scout frontier error = %v", err)
	}
	request.SeedPacks.Omissions = []themestudy.Omission{{
		Reason: "seed_budget", Count: 3, Representatives: []string{"a2", "a3", "a4"},
	}}
	if err := validateThemeScoutRequestAgainstInput(request, input); err != nil {
		t.Fatalf("honestly bounded Scout frontier: %v", err)
	}
	request.SeedPacks.Packs = []themestudy.SeedPack{pack(1), pack(2)}
	request.SeedPacks.Packs[0].Seed.CanonicalSpanID, request.SeedPacks.Packs[1].Seed.CanonicalSpanID =
		request.SeedPacks.Packs[1].Seed.CanonicalSpanID, request.SeedPacks.Packs[0].Seed.CanonicalSpanID
	request.SeedPacks.Omissions = []themestudy.Omission{{
		Reason: "seed_budget", Count: 2, Representatives: []string{"a3", "a4"},
	}}
	if err := validateThemeScoutRequestAgainstInput(request, input); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("swapped private span bindings error = %v", err)
	}
	request.SeedPacks.Packs = []themestudy.SeedPack{pack(2), pack(1)}
	request.SeedPacks.Omissions = []themestudy.Omission{{
		Reason: "seed_budget", Count: 2, Representatives: []string{"a3", "a4"},
	}}
	if err := validateThemeScoutRequestAgainstInput(request, input); err == nil ||
		!strings.Contains(err.Error(), "public API order") {
		t.Fatalf("reordered Scout roots error = %v", err)
	}
}

func d277LibraryReportData(t *testing.T, declarations []gofacts.PackageDeclaration) *ReportData {
	t.Helper()
	declarations, err := gofacts.CanonicalPackageDeclarations(declarations)
	if err != nil {
		t.Fatal(err)
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: "gopkg.in/telebot.v3", ModuleDir: ".", Main: true,
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "gopkg.in/telebot.v3", Name: "telebot", ModuleID: "module-root",
			ModulePath: "gopkg.in/telebot.v3", PackageDir: ".", ModuleRelativeDir: ".",
			Locality: "local", Files: []string{"assembly.go", "bot.go", "client.go", "download.go"},
			Declarations: declarations, DeclarationsScanned: true, LoadCompleteness: completeReportPackageLoad(),
		}},
	}
	resolution, err := analysistarget.Resolve(facts, analysistarget.Options{})
	if err != nil || resolution.Selected == nil {
		t.Fatalf("resolve library target: resolution=%#v err=%v", resolution, err)
	}
	target := resolution.Selected.Snapshot()
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "unit-repository", Kind: repositoryatlas.UnitRepository, Name: "repository"},
			{ID: "module-root", Kind: repositoryatlas.UnitModule, ParentID: "unit-repository", Name: "gopkg.in/telebot.v3"},
			{ID: "unit-package", Kind: repositoryatlas.UnitPackage, ParentID: "module-root", Name: "gopkg.in/telebot.v3"},
		},
	}
	return &ReportData{AnalysisTarget: &target, RepositoryAtlas: &atlas, repositoryGoFacts: &facts}
}

func completeReportPackageLoad() *gofacts.PackageLoadCompleteness {
	return &gofacts.PackageLoadCompleteness{
		Version: gofacts.PackageLoadCompletenessVersion,
		State:   gofacts.PackageLoadComplete,
	}
}
