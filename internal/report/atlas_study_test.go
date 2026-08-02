package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/repositoryatlas/goadapter"
)

func TestAtlasStudyCasdoorShapedPackageDrawerSourcesDoNotExpandReadingCatalog(t *testing.T) {
	data := atlasStudyReportFixture(t)
	baseline, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.ReadingTargets) != 3 {
		t.Fatalf("baseline reading targets = %d", len(baseline.ReadingTargets))
	}

	facts := gofacts.Facts{Modules: []gofacts.ModuleFact{{
		ID: "unit-module-fixture", ModulePath: "github.com/casdoor/casdoor", ModuleDir: ".",
	}}}
	declarations := make(map[string]evidence.Location)
	for index := 0; index < 34; index++ {
		name := fmt.Sprintf("pkg%02d", index)
		canonicalPath := "github.com/casdoor/casdoor/" + name
		sourcePath := name + "/package.go"
		facts.Packages = append(facts.Packages, gofacts.PackageFact{
			CanonicalPath: canonicalPath, Name: name,
			ModuleID: "unit-module-fixture", ModulePath: "github.com/casdoor/casdoor",
			Files: []string{sourcePath},
		})
		declarations[canonicalPath] = evidence.Location{Path: sourcePath, Line: 15, Column: 1}
		data.OpenablePaths = append(data.OpenablePaths, sourcePath)
	}
	projected, err := goadapter.Project(goadapter.Input{
		RepositoryName: "github.com/casdoor/casdoor", Facts: facts,
		PackageDeclarations: declarations,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceByPath := make(map[string]repositoryatlas.Evidence)
	for _, unit := range projected.Units {
		if unit.Kind == repositoryatlas.UnitPackage {
			data.RepositoryAtlas.Units = append(data.RepositoryAtlas.Units, unit)
		}
	}
	for _, item := range projected.Evidence {
		if item.Provenance.Operation == goadapter.PackageDeclarationEvidenceOperation {
			data.RepositoryAtlas.Evidence = append(data.RepositoryAtlas.Evidence, item)
			evidenceByPath[item.Location.Path] = item
		}
	}
	for _, pkg := range facts.Packages {
		item, ok := evidenceByPath[pkg.Files[0]]
		if !ok {
			t.Fatalf("package evidence missing for %s", pkg.CanonicalPath)
		}
		source := atlasStudySourceFixture(
			t, item.Location.Path, item.Location.Line, pkg.Name, "package "+pkg.Name,
		)
		source.RelatedEvidenceIDs = []string{item.ID}
		source.PresentationSHA256 = sourceSnippetPresentationSHA(source)
		data.UserSources = append(data.UserSources, source)
	}

	withPackageDrawers, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if len(withPackageDrawers.ReadingTargets) != len(baseline.ReadingTargets) {
		t.Fatalf("34 package drawer sources changed Study targets: before=%d after=%d",
			len(baseline.ReadingTargets), len(withPackageDrawers.ReadingTargets))
	}
	assertAtlasStudyJSONReplayParity(t, data, withPackageDrawers)

	data.UserSources[4].RelatedEvidenceIDs = append(
		data.UserSources[4].RelatedEvidenceIDs, "evidence-fixture-source",
	)
	data.UserSources[4].PresentationSHA256 = sourceSnippetPresentationSHA(data.UserSources[4])
	data.UserSources[5].RelatedEvidenceIDs = append(
		data.UserSources[5].RelatedEvidenceIDs, "unknown-evidence-id",
	)
	data.UserSources[5].PresentationSHA256 = sourceSnippetPresentationSHA(data.UserSources[5])
	for _, source := range data.UserSources[3:6] {
		line := atlasStudySourceFocusLine(source)
		data.ArchitectureCanvas.Components[0].Members[0].Facts = append(
			data.ArchitectureCanvas.Components[0].Members[0].Facts,
			componentmap.LocalFact{
				Kind: componentmap.FactRepositoryPath, Value: source.Path,
				Location: &evidence.Location{Path: source.Path, Line: line},
			},
		)
	}
	mixed, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if len(mixed.ReadingTargets) != len(baseline.ReadingTargets)+3 {
		t.Fatalf("mixed/unknown related identities erased eligible sources: before=%d after=%d",
			len(baseline.ReadingTargets), len(mixed.ReadingTargets))
	}
	assertAtlasStudyJSONReplayParity(t, data, mixed)

	withoutGraph := *data
	withoutGraph.RepositoryGraph = nil
	withoutGraphInput, err := BuildAtlasStudyInput(&withoutGraph, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("self-contained package evidence required capped RepositoryGraph: %v", err)
	}
	if !reflect.DeepEqual(withoutGraphInput.ReadingTargets, mixed.ReadingTargets) {
		t.Fatalf("RepositoryGraph changed package-evidence Study filtering")
	}
}

func assertAtlasStudyJSONReplayParity(
	t *testing.T,
	data *ReportData,
	want atlasstudy.Input,
) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var replayed ReportData
	if err := json.Unmarshal(raw, &replayed); err != nil {
		t.Fatal(err)
	}
	got, err := BuildAtlasStudyInput(&replayed, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("JSON replay BuildAtlasStudyInput: %v", err)
	}
	if !reflect.DeepEqual(got.ReadingTargets, want.ReadingTargets) {
		t.Fatalf("runtime/replay reading targets differ:\nruntime: %#v\nreplay:  %#v",
			want.ReadingTargets, got.ReadingTargets)
	}
}

func TestBuildAtlasStudyInputUsesExactAtlasArchitectureAndSavedSource(t *testing.T) {
	data := atlasStudyReportFixture(t)
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}
	if len(input.Surfaces) != 1 || input.Surfaces[0].ID != "surface-fixture-1" ||
		input.Surfaces[0].Authority != "resolved" || len(input.ReadingTargets) != 3 {
		t.Fatalf("exact Surface/source projection = %#v / %#v", input.Surfaces, input.ReadingTargets)
	}
	var entryTarget *atlasstudy.ReadingTarget
	for _, target := range input.ReadingTargets {
		if target.Owner != (atlasstudy.CanonicalRef{}) || target.Authority != repositoryatlas.AuthorityObserved ||
			len(target.RelatedComponentIDs) != 1 || target.RelatedComponentIDs[0] != "component-fixture-app" ||
			!slicesContainCanonicalRef(target.PrincipalRefs, atlasstudy.CanonicalRef{
				Kind: atlasstudy.RefComponent, ID: "component-fixture-app",
			}) {
			t.Fatalf("locator associations = %#v", target)
		}
		if target.Location.Path == "cmd/app/main.go" {
			targetCopy := target
			entryTarget = &targetCopy
		}
	}
	if entryTarget == nil || !slicesContainCanonicalRef(entryTarget.PrincipalRefs, atlasstudy.CanonicalRef{
		Kind: atlasstudy.RefSurface, ID: "surface-fixture-1",
	}) || entryTarget.Kind != atlasstudy.ReadingTargetEntrypoint {
		t.Fatalf("same locator did not retain Surface and Component associations: %#v", entryTarget)
	}
	if len(input.Evidence) != 1 || input.Evidence[0].ID != "evidence-fixture-source" {
		t.Fatalf("Atlas evidence projection = %#v", input.Evidence)
	}
	if len(input.Documents) != 1 || input.Documents[0].Claim != "Fixture accepts requests and returns a visible result." {
		t.Fatalf("documented claim = %#v", input.Documents)
	}

	product, err := atlasstudy.Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wire := product.WireJSON()
	for _, private := range [][]byte{
		[]byte(`"component-fixture-app"`),
		[]byte(`"surface-fixture-1"`),
	} {
		if bytes.Contains(wire, private) {
			t.Fatalf("provider wire exposed private identity %q: %s", private, wire)
		}
	}
	if !bytes.Contains(wire, []byte(`"reading_targets"`)) ||
		!bytes.Contains(wire, []byte(`"documented_claims"`)) ||
		!bytes.Contains(wire, []byte(`"allowed_paths":["cmd/app/main.go","internal/app/result.go","internal/app/run.go"]`)) ||
		!bytes.Contains(wire, []byte(`"path":"cmd/app/main.go","line":7,"symbol":"example.com/fixture/cmd/app.main"`)) {
		t.Fatalf("provider wire lost bounded Study facts: %s", wire)
	}
}

func TestAtlasStudyReadingTargetContextUsesExactSavedGoSyntax(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		line       string
		wantSymbol string
		wantKind   atlasstudy.ReadingTargetKind
		wantLabel  string
		wantFact   string
	}{
		{
			name: "function declaration", line: "func Start() {", wantSymbol: "Start",
			wantKind: atlasstudy.ReadingTargetFunction, wantLabel: "Function Start",
			wantFact: "Exact saved Go function declaration.",
		},
		{
			name: "method declaration", line: "func (server *Server) Start() {", wantSymbol: "Start",
			wantKind: atlasstudy.ReadingTargetMethod, wantLabel: "Method Start",
			wantFact: "Exact saved Go method declaration.",
		},
		{
			name:       "qualified package call",
			line:       "err := http.ListenAndServe(fmt.Sprintf(\":%d\", port), mux)",
			wantSymbol: "http.ListenAndServe", wantKind: atlasstudy.ReadingTargetFile,
			wantLabel: "Qualified Go call site", wantFact: "Exact saved source at a qualified Go call.",
		},
		{
			name: "qualified receiver call", line: "err := server.ListenAndServeTLS(\"\", \"\")",
			wantSymbol: "server.ListenAndServeTLS", wantKind: atlasstudy.ReadingTargetFile,
			wantLabel: "Qualified Go call site", wantFact: "Exact saved source at a qualified Go call.",
		},
		{
			name: "type declaration is not a function", line: "type Server struct {}",
			wantKind: atlasstudy.ReadingTargetFile, wantLabel: "Repository source",
			wantFact: "Exact saved repository source.",
		},
		{
			name: "nested selector is not the direct call", line: "serve(fmt.Sprintf(\"%d\", port))",
			wantKind: atlasstudy.ReadingTargetFile, wantLabel: "Repository source",
			wantFact: "Exact saved repository source.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := atlasStudySourceFixture(t, "service/proxy.go", 42, "", test.line)
			symbol, kind, label, fact := atlasStudyReadingTargetContext(source, 42, false, nil)
			if symbol != test.wantSymbol || kind != test.wantKind || label != test.wantLabel || fact != test.wantFact {
				t.Fatalf("context = %q, %q, %q, %q", symbol, kind, label, fact)
			}
		})
	}
}

func TestBuildAtlasStudyInputOmitsSurfaceWithoutExactAtlasEntity(t *testing.T) {
	data := atlasStudyReportFixture(t)
	data.ArchitectureCanvas.Surfaces[0].ID = "not-in-atlas"
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}
	if len(input.Surfaces) != 0 {
		t.Fatalf("unjoined Surfaces = %#v", input.Surfaces)
	}
	if len(input.ReadingTargets) != 3 {
		t.Fatalf("source target did not fall back to its exact component owner: %#v", input.ReadingTargets)
	}
	for _, target := range input.ReadingTargets {
		if target.Owner != (atlasstudy.CanonicalRef{}) || len(target.PrincipalRefs) != 1 ||
			target.PrincipalRefs[0] != (atlasstudy.CanonicalRef{
				Kind: atlasstudy.RefComponent, ID: "component-fixture-app",
			}) {
			t.Fatalf("unexpected unjoined associations: %#v", target)
		}
	}
}

func TestBuildAtlasStudyInputDeduplicatesEquivalentLocatorAndOmitsConflict(t *testing.T) {
	t.Run("equivalent saved source", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		data.UserSources = append(data.UserSources, data.UserSources[0])
		input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
		if err != nil {
			t.Fatalf("BuildAtlasStudyInput: %v", err)
		}
		if len(input.ReadingTargets) != 3 || !AtlasStudyInputHasMinimumCatalog(input) {
			t.Fatalf("equivalent source changed exact catalog: %#v", input.ReadingTargets)
		}
	})

	t.Run("benign broader overlap", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		data.UserSources = append(data.UserSources, atlasStudySourceRangeFixture(
			t,
			"cmd/app/main.go",
			6,
			8,
			7,
			"example.com/fixture/cmd/app.main",
			[]string{"package main", "func main() {}", ""},
		))
		input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
		if err != nil {
			t.Fatalf("BuildAtlasStudyInput: %v", err)
		}
		if len(input.ReadingTargets) != 3 || !AtlasStudyInputHasMinimumCatalog(input) {
			t.Fatalf("broader exact context changed reading locations: %#v", input.ReadingTargets)
		}
	})

	t.Run("conflicting saved source", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		conflicting := atlasStudySourceFixture(
			t,
			data.UserSources[0].Path,
			atlasStudySourceFocusLine(data.UserSources[0]),
			data.UserSources[0].EnclosingSymbol,
			"func main() { panic(\"conflicting saved state\") }",
		)
		data.UserSources = append(data.UserSources, conflicting)
		input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
		if err != nil {
			t.Fatalf("BuildAtlasStudyInput: %v", err)
		}
		if len(input.ReadingTargets) != 2 || AtlasStudyInputHasMinimumCatalog(input) {
			t.Fatalf("conflicting source locator remained model-visible: %#v", input.ReadingTargets)
		}
		for _, target := range input.ReadingTargets {
			if target.Location.Path == data.UserSources[0].Path {
				t.Fatalf("conflicted locator survived as reading target: %#v", target)
			}
		}
	})

	t.Run("conflicting empty-symbol source", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		original := data.UserSources[0]
		first := atlasStudySourceFixture(
			t, original.Path, atlasStudySourceFocusLine(original), "", "func Start() {}",
		)
		second := atlasStudySourceFixture(
			t, original.Path, atlasStudySourceFocusLine(original), "", "func Stop() {}",
		)
		data.UserSources[0] = first
		data.UserSources = append(data.UserSources, second)
		input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
		if err != nil {
			t.Fatalf("BuildAtlasStudyInput: %v", err)
		}
		for _, target := range input.ReadingTargets {
			if target.Location.Path == original.Path &&
				target.Location.Line == atlasStudySourceFocusLine(original) {
				t.Fatalf("conflicting empty-symbol sources split into targets: %#v", input.ReadingTargets)
			}
		}
	})
}

func TestBuildAtlasStudyInputKeepsOneLocatorWithPluralConceptualMembership(t *testing.T) {
	data := atlasStudyReportFixture(t)
	secondSubsystemID := componentmap.SubsystemID("subsystem-cross-cutting")
	secondComponentID := componentmap.ComponentID("component-cross-cutting")
	data.ArchitectureCanvas.Subsystems = append(data.ArchitectureCanvas.Subsystems, ArchitectureSubsystem{
		ID: secondSubsystemID, Name: "Cross-cutting concerns",
		ComponentIDs: []componentmap.ComponentID{secondComponentID},
	})
	data.ArchitectureCanvas.Components = append(data.ArchitectureCanvas.Components, ArchitectureComponent{
		ID: secondComponentID, SubsystemID: secondSubsystemID, Name: "Startup policy",
		Members: []componentmap.Candidate{{
			ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: "member-cross-cutting"},
			Facts: []componentmap.LocalFact{{
				Kind: componentmap.FactRepositoryPath, Value: "cmd/app/main.go",
				Location: &evidence.Location{Path: "cmd/app/main.go", Line: 7},
			}},
		}},
	})

	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}
	var target *atlasstudy.ReadingTarget
	for index := range input.ReadingTargets {
		if input.ReadingTargets[index].Location.Path == "cmd/app/main.go" {
			target = &input.ReadingTargets[index]
			break
		}
	}
	if target == nil || target.Owner != (atlasstudy.CanonicalRef{}) ||
		!reflect.DeepEqual(target.RelatedComponentIDs, []string{
			"component-cross-cutting", "component-fixture-app",
		}) ||
		!slicesContainCanonicalRef(target.PrincipalRefs, atlasstudy.CanonicalRef{
			Kind: atlasstudy.RefComponent, ID: string(secondComponentID),
		}) {
		t.Fatalf("one exact locator lost plural conceptual membership: %#v", target)
	}
	if len(input.ReadingTargets) != 3 {
		t.Fatalf("conceptual membership cloned reading location: %#v", input.ReadingTargets)
	}
}

func TestBuildAtlasStudyInputOwnerRequiresOneExactProducerProof(t *testing.T) {
	t.Run("one exact producer", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		data.ArchitectureCanvas.Surfaces[0].OwningComponentID = "component-fixture-app"
		input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
		if err != nil {
			t.Fatalf("BuildAtlasStudyInput: %v", err)
		}
		for _, target := range input.ReadingTargets {
			if target.Location.Path == "cmd/app/main.go" && target.Owner != (atlasstudy.CanonicalRef{
				Kind: atlasstudy.RefComponent, ID: "component-fixture-app",
			}) {
				t.Fatalf("exact Surface producer was not restored: %#v", target)
			}
		}
	})

	t.Run("conflicting exact producers", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		secondSubsystemID := componentmap.SubsystemID("subsystem-second")
		secondComponentID := componentmap.ComponentID("component-second")
		data.ArchitectureCanvas.Subsystems = append(data.ArchitectureCanvas.Subsystems, ArchitectureSubsystem{
			ID: secondSubsystemID, Name: "Second", ComponentIDs: []componentmap.ComponentID{secondComponentID},
		})
		data.ArchitectureCanvas.Components = append(data.ArchitectureCanvas.Components, ArchitectureComponent{
			ID: secondComponentID, SubsystemID: secondSubsystemID, Name: "Second component",
		})
		data.ArchitectureCanvas.Surfaces[0].OwningComponentID = "component-fixture-app"
		data.ArchitectureCanvas.Flows = []ArchitectureFlow{{
			ID: "flow-conflicting-owner", Name: "Conflicting owner proof",
			Steps: []ArchitectureFlowStep{{
				ID: "step-conflicting-owner", ComponentID: secondComponentID,
				Location: &evidence.Location{Path: "cmd/app/main.go", Line: 7},
			}},
		}}
		input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
		if err != nil {
			t.Fatalf("BuildAtlasStudyInput: %v", err)
		}
		for _, target := range input.ReadingTargets {
			if target.Location.Path != "cmd/app/main.go" {
				continue
			}
			if target.Owner != (atlasstudy.CanonicalRef{}) ||
				!slicesContainCanonicalRef(target.PrincipalRefs, atlasstudy.CanonicalRef{
					Kind: atlasstudy.RefComponent, ID: string(secondComponentID),
				}) {
				t.Fatalf("conflicting exact producer proof chose an owner: %#v", target)
			}
		}
	})
}

func TestReadAtlasStudyReportProductAcceptedProjectsExactSources(t *testing.T) {
	data := atlasStudyReportFixture(t)
	runDir := t.TempDir()
	writeAcceptedAtlasStudyArtifacts(t, runDir, data)
	status, studyMap, err := readAtlasStudyReportProduct(runDir, data)
	if err != nil {
		t.Fatalf("readAtlasStudyReportProduct: %v", err)
	}
	if status == nil || status.State != atlasstudy.ProductStateAccepted ||
		status.DirectionCount != 1 || studyMap == nil || len(studyMap.Directions) != 1 ||
		len(studyMap.Directions[0].ReadingAnchors) != 3 || len(studyMap.Shape) != 1 {
		t.Fatalf("accepted report projection = %#v / %#v", status, studyMap)
	}
	for _, reading := range studyMap.Directions[0].ReadingAnchors {
		if reading.Source.Path != reading.Location.Path || reading.Source.Validate() != nil {
			t.Fatalf("reading lost exact saved source: %#v", reading)
		}
	}

	data.DocumentedPurpose += " Changed after the request was saved."
	if _, _, err := readAtlasStudyReportProduct(runDir, data); err == nil {
		t.Fatal("request bound to prior report input was accepted after purpose tamper")
	}
}

func TestReadAtlasStudyReportProductResolvesDerivedExactTargetContext(t *testing.T) {
	data := atlasStudyReportFixture(t)
	for index := range data.UserSources {
		if data.UserSources[index].Path == "internal/app/run.go" {
			data.UserSources[index] = atlasStudySourceFixture(
				t, "internal/app/run.go", 11, "",
				"err := http.ListenAndServe(\":8080\", mux)",
			)
		}
	}
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}
	found := false
	for _, target := range input.ReadingTargets {
		if target.Location.Path == "internal/app/run.go" {
			found = target.Symbol == "http.ListenAndServe" &&
				target.Label == "Qualified Go call site"
		}
	}
	if !found {
		t.Fatalf("derived exact target context = %#v", input.ReadingTargets)
	}
	runDir := t.TempDir()
	writeAcceptedAtlasStudyArtifacts(t, runDir, data)
	if _, studyMap, err := readAtlasStudyReportProduct(runDir, data); err != nil || studyMap == nil {
		t.Fatalf("derived exact target report replay = %#v, %v", studyMap, err)
	}
}

func TestProjectAtlasStudyShapeSourceRequiresOneExactOwnedLocator(t *testing.T) {
	t.Run("one exact producer location", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		data.ArchitectureCanvas.Surfaces[0].OwningComponentID = "component-fixture-app"
		runDir := t.TempDir()
		writeAcceptedAtlasStudyArtifacts(t, runDir, data)
		_, studyMap, err := readAtlasStudyReportProduct(runDir, data)
		if err != nil {
			t.Fatalf("read accepted product: %v", err)
		}
		if len(studyMap.Shape) != 1 || studyMap.Shape[0].CodeLocation == nil ||
			studyMap.Shape[0].Source == nil ||
			studyMap.Shape[0].CodeLocation.Path != "cmd/app/main.go" {
			t.Fatalf("sole exact owner was not projected: %#v", studyMap.Shape)
		}
	})

	t.Run("multiple exact producer locations", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		data.ArchitectureCanvas.Surfaces[0].OwningComponentID = "component-fixture-app"
		data.ArchitectureCanvas.Flows = []ArchitectureFlow{{
			ID: "flow-second-owned-locator", Name: "Second exact owned locator",
			Steps: []ArchitectureFlowStep{{
				ID: "step-second-owned-locator", ComponentID: "component-fixture-app",
				Location: &evidence.Location{Path: "internal/app/run.go", Line: 11},
			}},
		}}
		runDir := t.TempDir()
		writeAcceptedAtlasStudyArtifacts(t, runDir, data)
		_, studyMap, err := readAtlasStudyReportProduct(runDir, data)
		if err != nil {
			t.Fatalf("read accepted product: %v", err)
		}
		if len(studyMap.Shape) != 1 || studyMap.Shape[0].CodeLocation != nil ||
			studyMap.Shape[0].Source != nil {
			t.Fatalf("conceptual Shape selected an arbitrary owned source: %#v", studyMap.Shape)
		}
	})
}

func TestReadAtlasStudyReportProductTerminalStateMatrix(t *testing.T) {
	t.Run("failed called stage", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		runDir := t.TempDir()
		product := compileAtlasStudyFixture(t, data)
		writeAtlasStudyRequest(t, runDir, product)
		status, err := product.FailureStatus(atlasstudy.FailureProvider)
		if err != nil {
			t.Fatalf("FailureStatus: %v", err)
		}
		writeAtlasStudyStatus(t, runDir, status)
		projected, studyMap, err := readAtlasStudyReportProduct(runDir, data)
		if err != nil || projected == nil || projected.State != atlasstudy.ProductStateFailed ||
			projected.FailureCode != atlasstudy.FailureProvider || studyMap != nil {
			t.Fatalf("failed projection = %#v / %#v / %v", projected, studyMap, err)
		}
	})

	t.Run("offline uncalled stage", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		data.ArchitectureSynthesis = &ArchitectureSynthesisStatus{
			Version: ArchitectureSynthesisStatusVersion, State: ArchitectureSynthesisUnavailable,
			UnavailableCode: "offline",
		}
		projected, studyMap, err := readAtlasStudyReportProduct(t.TempDir(), data)
		if err != nil || projected == nil || projected.State != atlasstudy.ProductStateUnavailable ||
			projected.UnavailableCode != AtlasStudyUnavailableOffline || studyMap != nil {
			t.Fatalf("offline projection = %#v / %#v / %v", projected, studyMap, err)
		}
	})

	t.Run("partial artifact set", func(t *testing.T) {
		data := atlasStudyReportFixture(t)
		runDir := t.TempDir()
		writeAtlasStudyRequest(t, runDir, compileAtlasStudyFixture(t, data))
		if _, _, err := readAtlasStudyReportProduct(runDir, data); err == nil {
			t.Fatal("request without terminal status was accepted")
		}
	})
}

func TestReplayAcceptedArchitectureRequiresStatusArtifactAgreement(t *testing.T) {
	for _, test := range []struct {
		name   string
		status ArchitectureSynthesisStatus
	}{
		{name: "failed", status: ArchitectureSynthesisStatus{State: ArchitectureSynthesisFailed}},
		{name: "offline", status: ArchitectureSynthesisStatus{
			State: ArchitectureSynthesisUnavailable, UnavailableCode: "offline",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			canvas := &ArchitectureCanvas{ArchitectureSource: componentmap.SourceLocalPackages}
			data := &ReportData{ArchitectureCanvas: canvas, ArchitectureSynthesis: &test.status}
			if err := replayAcceptedArchitectureForReport(data, t.TempDir(), nil); err != nil {
				t.Fatalf("terminal status without synthesis: %v", err)
			}
			if data.ArchitectureCanvas != canvas {
				t.Fatal("terminal model status replaced the local Architecture Canvas")
			}
			if err := replayAcceptedArchitectureForReport(
				data, t.TempDir(), &savedArchitectureArtifacts{synthesis: []byte(`{}`)},
			); err == nil {
				t.Fatal("terminal model status authorized a stale synthesis")
			}
		})
	}
}

func TestRunManifestVerifiesAtlasStudyProjectionAndTampering(t *testing.T) {
	data := atlasStudyReportFixture(t)
	runDir := t.TempDir()
	writeAcceptedAtlasStudyArtifacts(t, runDir, data)
	status, studyMap, err := readAtlasStudyReportProduct(runDir, data)
	if err != nil {
		t.Fatalf("read accepted product: %v", err)
	}
	data.FormatVersion = CurrentFormatVersion
	data.AtlasStudy, data.StudyMap = status, studyMap
	reportJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	manifest := RunManifest{Version: CurrentRunManifestVersion, MaterialInputs: MaterialInputs{
		AtlasStudyRequestSHA256: manifestSHA256(mustReadAtlasStudyFile(t, runDir, atlasstudy.RequestArtifactFilename)),
		AtlasStudyResultSHA256:  manifestSHA256(mustReadAtlasStudyFile(t, runDir, atlasstudy.ResultArtifactFilename)),
		AtlasStudyStatusSHA256:  manifestSHA256(mustReadAtlasStudyFile(t, runDir, atlasstudy.StatusArtifactFilename)),
	}}
	if err := manifest.VerifyAtlasStudyArtifacts(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyAtlasStudyArtifacts: %v", err)
	}

	var tampered ReportData
	if err := json.Unmarshal(reportJSON, &tampered); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	tampered.StudyMap.Directions[0].Question = "A different unsupported report question?"
	tamperedJSON, _ := json.Marshal(tampered)
	if err := manifest.VerifyAtlasStudyArtifacts(runDir, tamperedJSON); err == nil {
		t.Fatal("tampered report projection matched exact Atlas Study artifacts")
	}

	resultPath := filepath.Join(runDir, atlasstudy.ResultArtifactFilename)
	resultRaw := mustReadAtlasStudyFile(t, runDir, atlasstudy.ResultArtifactFilename)
	if err := os.WriteFile(resultPath, append(resultRaw, ' '), 0o600); err != nil {
		t.Fatalf("tamper result: %v", err)
	}
	if err := manifest.VerifyAtlasStudyArtifacts(runDir, reportJSON); err == nil {
		t.Fatal("tampered result bytes matched manifest")
	}
}

func TestRunManifestAcceptsInsufficientCatalogIndependentlyOfArchitectureEnrichment(t *testing.T) {
	atlas := repositoryAtlasWithoutStartup()
	atlasJSON, err := repositoryatlas.CanonicalJSON(atlas)
	if err != nil {
		t.Fatal(err)
	}
	navigatorFixture := makeNavigatorArtifactFixture(t, atlas, navigator.ProductStateEmpty)
	reportJSON, err := json.Marshal(&ReportData{
		FormatVersion: CurrentFormatVersion, RepositoryAtlas: &atlas,
		Navigator: &navigatorFixture.projection,
		AtlasStudy: &AtlasStudyReportStatus{
			Version: atlasstudy.Version, State: atlasstudy.ProductStateUnavailable,
			UnavailableCode: AtlasStudyUnavailableInsufficientCatalog,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := validRunManifestFixture(t)
	manifest.OpenablePaths, manifest.Components = nil, nil
	manifest.ReportSHA256 = manifestSHA256(reportJSON)
	manifest.MaterialInputs.RepositoryAtlasSHA256 = manifestSHA256(atlasJSON)
	manifest.MaterialInputs.NavigatorResultSHA256 = manifestSHA256(navigatorFixture.result)
	manifest.MaterialInputs.NavigatorStatusSHA256 = manifestSHA256(navigatorFixture.status)
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		t.Fatalf("insufficient exact Study catalog report rejected: %v", err)
	}
}

func TestReadAtlasStudyReportProductRejectsObviousCredentialWithoutEcho(t *testing.T) {
	runDir := t.TempDir()
	const secret = "actual-secret-value"
	if err := os.WriteFile(
		filepath.Join(runDir, atlasstudy.RequestArtifactFilename),
		[]byte(`{"api_key":"`+secret+`"}`), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	_, _, err := readAtlasStudyReportProduct(runDir, atlasStudyReportFixture(t))
	if err == nil || !strings.Contains(err.Error(), "obvious credential") ||
		strings.Contains(err.Error(), secret) {
		t.Fatalf("credential rejection = %v", err)
	}
}

func TestPrepareAuthorizedSourceCoverageMakesCasdoorShapedStudyReplayable(t *testing.T) {
	authority := overviewSourceCoverageAuthority(t, map[string]map[int]string{
		"main.go":   {7: "func main() {}"},
		"run.go":    {11: "func Run() {}"},
		"result.go": {19: "func Result() {}"},
	})
	data := atlasStudyReportFixture(t)
	data.UserSources = nil
	data.OpenablePaths = []string{"main.go", "result.go", "run.go"}
	data.RepositoryAtlas.Evidence[0].Location.Path = "main.go"
	data.ArchitectureCanvas.Surfaces[0].Evidence[0].Path = "main.go"
	paths := []string{"main.go", "run.go", "result.go"}
	lines := []int{7, 11, 19}
	for index := range data.ArchitectureCanvas.Components[0].Members[0].Facts {
		fact := &data.ArchitectureCanvas.Components[0].Members[0].Facts[index]
		fact.Value = paths[index]
		fact.Location = &evidence.Location{Path: paths[index], Line: lines[index]}
	}
	if err := PrepareAuthorizedSourceCoverage(context.Background(), data, &authority); err != nil {
		t.Fatalf("PrepareAuthorizedSourceCoverage: %v", err)
	}
	if len(data.UserSources) < 3 {
		t.Fatalf("authorized Casdoor-shaped source targets = %d, want at least 3", len(data.UserSources))
	}
	first, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput after source coverage: %v", err)
	}
	if len(first.ReadingTargets) < 3 {
		t.Fatalf("Atlas Study reading targets = %d, want at least 3", len(first.ReadingTargets))
	}
	if _, err := atlasstudy.Compile(first); err != nil {
		t.Fatalf("Compile authorized input: %v", err)
	}
	if err := PrepareAuthorizedSourceCoverage(context.Background(), data, &authority); err != nil {
		t.Fatalf("idempotent PrepareAuthorizedSourceCoverage: %v", err)
	}
	second, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("second BuildAtlasStudyInput: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("authorized exact source coverage changed Atlas Study replay identity")
	}
}

func compileAtlasStudyFixture(t *testing.T, data *ReportData) atlasstudy.Product {
	t.Helper()
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}
	product, err := atlasstudy.Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return product
}

func writeAcceptedAtlasStudyArtifacts(t *testing.T, runDir string, data *ReportData) {
	t.Helper()
	product := compileAtlasStudyFixture(t, data)
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("RequestRecord: %v", err)
	}
	componentRef := ""
	var targetRefs []string
	for _, object := range request.Catalog {
		if object.Kind == atlasstudy.RefComponent && object.CanonicalID == "component-fixture-app" {
			componentRef = object.Ref
		}
		if object.Kind == atlasstudy.RefReadingTarget &&
			slicesContainCanonicalRef(object.PrincipalRefs, atlasstudy.CanonicalRef{
				Kind: atlasstudy.RefComponent, ID: "component-fixture-app",
			}) {
			targetRefs = append(targetRefs, object.Ref)
		}
	}
	if componentRef == "" || len(targetRefs) < 3 {
		t.Fatalf("fixture catalog lacks component routes: %#v", request.Catalog)
	}
	statement := func(text string) map[string]any {
		return map[string]any{"text": text, "support_refs": []string{componentRef}}
	}
	reading := make([]map[string]any, 0, 3)
	labels := []string{"start", "continue", "verify"}
	for index, ref := range targetRefs[:3] {
		reading = append(reading, map[string]any{
			"target_ref": ref, "label": labels[index],
			"what_to_look_for": "Inspect the advertised local responsibility.",
		})
	}
	response, err := json.Marshal(map[string]any{
		"repository_type": "service_application",
		"brief": map[string]any{
			"what_it_is":             statement("A bounded fixture application."),
			"problem":                statement("It keeps a small example easy to inspect."),
			"main_input":             statement("A locally advertised application input."),
			"central_responsibility": statement("It handles the advertised fixture responsibility."),
			"observable_result":      statement("It exposes a visible fixture result."),
		},
		"directions": []map[string]any{{
			"question":         "How is the fixture responsibility organized?",
			"why_it_matters":   "This identifies the bounded conceptual area.",
			"learning_outcome": "Recognize the exact saved reading anchors.",
			"target_job":       "first_contact", "learning_stage": "orientation",
			"principal_refs": []string{componentRef}, "reading": reading,
		}},
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	result, _, err := product.ResolveResponseJSON(response)
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	status, err := product.AcceptedStatus(result)
	if err != nil {
		t.Fatalf("AcceptedStatus: %v", err)
	}
	writeAtlasStudyRequest(t, runDir, product)
	resultRaw, err := atlasstudy.EncodeResultRecord(result)
	if err != nil {
		t.Fatalf("EncodeResultRecord: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, atlasstudy.ResultArtifactFilename), resultRaw, 0o600); err != nil {
		t.Fatalf("write result: %v", err)
	}
	writeAtlasStudyStatus(t, runDir, status)
}

func slicesContainCanonicalRef(values []atlasstudy.CanonicalRef, want atlasstudy.CanonicalRef) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeAtlasStudyRequest(t *testing.T, runDir string, product atlasstudy.Product) {
	t.Helper()
	record, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("RequestRecord: %v", err)
	}
	raw, err := atlasstudy.EncodeRequestRecord(record)
	if err != nil {
		t.Fatalf("EncodeRequestRecord: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, atlasstudy.RequestArtifactFilename), raw, 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func writeAtlasStudyStatus(t *testing.T, runDir string, status atlasstudy.Status) {
	t.Helper()
	raw, err := atlasstudy.EncodeStatus(status)
	if err != nil {
		t.Fatalf("EncodeStatus: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, atlasstudy.StatusArtifactFilename), raw, 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
}

func mustReadAtlasStudyFile(t *testing.T, runDir string, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return raw
}

func atlasStudyReportFixture(t *testing.T) *ReportData {
	t.Helper()
	sources := []SourceSnippet{
		atlasStudySourceFixture(t, "cmd/app/main.go", 7, "example.com/fixture/cmd/app.main", "func main() {}"),
		atlasStudySourceFixture(t, "internal/app/run.go", 11, "example.com/fixture/internal/app.Run", "func Run() {}"),
		atlasStudySourceFixture(t, "internal/app/result.go", 19, "example.com/fixture/internal/app.Result", "func Result() {}"),
	}
	atlas := repositoryAtlasFixture()
	atlas.Units[0].ID = "unit-repository-fixture"
	atlas.Units[1].ID = "unit-module-fixture"
	atlas.Units[1].ParentID = atlas.Units[0].ID
	atlas.Units[2].ID = "unit-app-fixture"
	atlas.Units[2].ParentID = atlas.Units[1].ID
	atlas.Entities[0].ID = "surface-fixture-1"
	atlas.Entities[0].UnitID = atlas.Units[2].ID
	atlas.Entities[1].ID = "operation-fixture-1"
	atlas.Entities[1].UnitID = atlas.Units[2].ID
	atlas.Observations[0].ID = "observation-surface-fixture"
	atlas.Observations[0].UnitID = atlas.Units[2].ID
	atlas.Observations[0].Subject.ID = atlas.Entities[0].ID
	atlas.Observations[0].EvidenceRefs = []string{"evidence-fixture-source"}
	atlas.Observations[1].ID = "observation-operation-fixture"
	atlas.Observations[1].UnitID = atlas.Units[2].ID
	atlas.Observations[1].Subject.ID = atlas.Entities[1].ID
	atlas.Observations[1].EvidenceRefs = []string{"evidence-fixture-source"}
	atlas.Evidence[0].ID = "evidence-fixture-source"
	atlas.Evidence[0].UnitID = atlas.Units[2].ID
	atlas.Relations[0].ID = "relation-fixture-startup"
	atlas.Relations[0].UnitID = atlas.Units[2].ID
	atlas.Relations[0].Source.ID = atlas.Entities[0].ID
	atlas.Relations[0].Target.ID = atlas.Entities[1].ID
	atlas.Relations[0].EvidenceRefs = []string{"evidence-fixture-source"}
	return &ReportData{
		RepoName:          "example.com/fixture",
		DocumentedPurpose: "Fixture accepts requests and returns a visible result.",
		RepositoryAtlas:   &atlas,
		OpenablePaths:     []string{"cmd/app/main.go", "internal/app/result.go", "internal/app/run.go"},
		UserSources:       sources,
		ArchitectureCanvas: &ArchitectureCanvas{
			Version:            ArchitectureCanvasVersion,
			ValidationOutcome:  componentmap.ValidationAccepted,
			ArchitectureSource: componentmap.SourceValidatedModel,
			ArchitectureLevel:  2,
			Title:              "Fixture architecture",
			Subsystems: []ArchitectureSubsystem{{
				ID: "subsystem-app", Name: "Application",
				ComponentIDs: []componentmap.ComponentID{"component-fixture-app"},
			}},
			Components: []ArchitectureComponent{{
				ID: "component-fixture-app", SubsystemID: "subsystem-app", Name: "Application entry",
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: "member-file"},
					Facts: []componentmap.LocalFact{
						{Kind: componentmap.FactRepositoryPath, Value: "cmd/app/main.go", Location: &evidence.Location{Path: "cmd/app/main.go", Line: 7}},
						{Kind: componentmap.FactRepositoryPath, Value: "internal/app/run.go", Location: &evidence.Location{Path: "internal/app/run.go", Line: 11}},
						{Kind: componentmap.FactRepositoryPath, Value: "internal/app/result.go", Location: &evidence.Location{Path: "internal/app/result.go", Line: 19}},
					},
				}},
			}},
			Surfaces: []ArchitectureSurface{{
				ID: "surface-fixture-1", Name: "Application startup", Kind: "process_entry",
				Resolution: "exact", Status: "confirmed_process_entry",
				Evidence: []SurfaceLocation{{Path: "cmd/app/main.go", Line: 7}},
			}},
		},
		ArchitectureSynthesis: func() *ArchitectureSynthesisStatus {
			status := architectureSynthesisV4AcceptedFixture()
			status.ArchitectureSource = string(componentmap.SourceValidatedModel)
			return &status
		}(),
	}
}

func atlasStudySourceFixture(
	t *testing.T,
	sourcePath string,
	line int,
	symbol string,
	content string,
) SourceSnippet {
	t.Helper()
	source := SourceSnippet{
		Path: sourcePath, Language: "go", EnclosingSymbol: symbol,
		StartLine: line, EndLine: line,
		HighlightRanges: []SourceHighlight{{StartLine: line, EndLine: line}},
		Content:         content,
		Lines:           []SourceSnippetLine{{Line: line, Text: content, Highlight: true}},
		ContentSHA256:   sourceLinesSHA256([]string{content}),
		Role:            "primary", SourceComplete: true,
	}
	source.PresentationSHA256 = sourceSnippetPresentationSHA(source)
	if err := source.Validate(); err != nil {
		t.Fatalf("source fixture: %v", err)
	}
	return source
}

func atlasStudySourceRangeFixture(
	t *testing.T,
	sourcePath string,
	startLine int,
	endLine int,
	highlightLine int,
	symbol string,
	content []string,
) SourceSnippet {
	t.Helper()
	if len(content) != endLine-startLine+1 {
		t.Fatalf("range fixture lines = %d, want %d", len(content), endLine-startLine+1)
	}
	lines := make([]SourceSnippetLine, 0, len(content))
	for index, text := range content {
		line := startLine + index
		lines = append(lines, SourceSnippetLine{
			Line: line, Text: text, Highlight: line == highlightLine,
		})
	}
	source := SourceSnippet{
		Path: sourcePath, Language: "go", EnclosingSymbol: symbol,
		StartLine: startLine, EndLine: endLine,
		HighlightRanges: []SourceHighlight{{StartLine: highlightLine, EndLine: highlightLine}},
		Content:         strings.Join(content, "\n"),
		Lines:           lines,
		ContentSHA256:   sourceLinesSHA256(content),
		Role:            "primary", SourceComplete: true,
	}
	source.PresentationSHA256 = sourceSnippetPresentationSHA(source)
	if err := source.Validate(); err != nil {
		t.Fatalf("range source fixture: %v", err)
	}
	return source
}
