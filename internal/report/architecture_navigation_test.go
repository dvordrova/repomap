package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

func TestProjectArchitectureComponentNavigationKeepsPluralSourcesAndMapOnlyComponents(t *testing.T) {
	t.Parallel()

	data := architectureComponentNavigationFixture()
	projection, err := ProjectArchitectureComponentNavigation(
		data.ArchitectureCanvas,
		data.OpenablePaths,
	)
	if err != nil {
		t.Fatalf("ProjectArchitectureComponentNavigation: %v", err)
	}
	if projection.Version != ArchitectureComponentNavigationVersion || len(projection.Components) != 2 {
		t.Fatalf("projection = %#v", projection)
	}

	symbols := projection.Components[0]
	if symbols.ComponentID != "component-symbols" ||
		symbols.MapTarget.Kind != SemanticSearchTargetComponent ||
		symbols.MapTarget.ComponentID != symbols.ComponentID {
		t.Fatalf("symbol component map target = %#v", symbols)
	}
	if len(symbols.PackageParticipantIDs) != 1 ||
		symbols.PackageParticipantIDs[0].Kind != componentmap.MemberPackage {
		t.Fatalf("package participants = %#v", symbols.PackageParticipantIDs)
	}
	if len(symbols.SymbolSources) != 2 ||
		symbols.SymbolSources[0].Symbol != "example.Service.Zebra" ||
		symbols.SymbolSources[1].Symbol != "example.Service.Alpha" ||
		symbols.SymbolSources[0].MemberID == symbols.SymbolSources[1].MemberID ||
		symbols.SymbolSources[0].Location.Path != symbols.SymbolSources[1].Location.Path {
		t.Fatalf("plural same-file symbol sources lost member order or identity: %#v", symbols.SymbolSources)
	}
	if symbols.SymbolSources[0].Location.Line != 30 || symbols.SymbolSources[1].Location.Line != 10 {
		t.Fatalf("symbol sources were sorted into presentation primacy: %#v", symbols.SymbolSources)
	}

	mapOnly := projection.Components[1]
	if mapOnly.ComponentID != "component-package-only" ||
		mapOnly.MapTarget.ComponentID != mapOnly.ComponentID ||
		len(mapOnly.SymbolSources) != 0 {
		t.Fatalf("package-only component is not map-navigable without a source: %#v", mapOnly)
	}
	for _, component := range projection.Components {
		if component.ComponentID == data.ArchitectureCanvas.LocalRemainderComponentID {
			t.Fatalf("local remainder leaked into accepted component navigation: %#v", projection)
		}
	}

	openable := map[string]struct{}{"batch.go": {}}
	locations := overviewArchitectureComponentLocations(
		data,
		data.ArchitectureCanvas.Components[0],
		openable,
	)
	if len(locations) != 2 || locations[0].Line != 30 || locations[1].Line != 10 {
		t.Fatalf("Overview source coverage reintroduced sorted-first semantics: %#v", locations)
	}
}

func TestRunManifestRejectsArchitectureComponentNavigationIdentityDrift(t *testing.T) {
	t.Parallel()

	base := architectureComponentNavigationFixture()
	base.FormatVersion = CurrentFormatVersion
	projection, err := ProjectArchitectureComponentNavigation(
		base.ArchitectureCanvas,
		base.OpenablePaths,
	)
	if err != nil {
		t.Fatal(err)
	}
	base.ArchitectureComponentNavigation = projection

	verify := func(t *testing.T, data *ReportData) error {
		t.Helper()
		reportJSON, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		manifest := validRunManifestFixture(t)
		manifest.Components = nil
		manifest.ReportSHA256 = manifestSHA256(reportJSON)
		return manifest.VerifyReportJSON(reportJSON)
	}
	if err := verify(t, base); err != nil {
		t.Fatalf("valid exact navigation rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ReportData)
	}{
		{
			name: "unknown component",
			mutate: func(data *ReportData) {
				data.ArchitectureComponentNavigation.Components[0].ComponentID = "component-unknown"
			},
		},
		{
			name: "unknown member",
			mutate: func(data *ReportData) {
				data.ArchitectureComponentNavigation.Components[0].SymbolSources[0].MemberID =
					componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "symbol-unknown"}
			},
		},
		{
			name: "source drift",
			mutate: func(data *ReportData) {
				data.ArchitectureComponentNavigation.Components[0].SymbolSources[0].Location.Line++
			},
		},
		{
			name: "broken typed ancestry",
			mutate: func(data *ReportData) {
				unknown := componentmap.MemberID{Kind: componentmap.MemberFile, Value: "file-unknown"}
				data.ArchitectureCanvas.Components[0].Members[1].ParentID = &unknown
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := cloneArchitectureNavigationReport(t, base)
			test.mutate(data)
			err := verify(t, data)
			if err == nil || !strings.Contains(err.Error(), "architecture component navigation") {
				t.Fatalf("VerifyReportJSON error = %v", err)
			}
		})
	}
}

func architectureComponentNavigationFixture() *ReportData {
	packageID := componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "package-service"}
	packageOnlyID := componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "package-only"}
	fileID := componentmap.MemberID{Kind: componentmap.MemberFile, Value: "file-service"}
	zebraID := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "symbol-zebra"}
	alphaID := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "symbol-alpha"}
	remainderID := componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "symbol-remainder"}
	fact := func(kind componentmap.FactKind, value string, line int) componentmap.LocalFact {
		return componentmap.LocalFact{
			Kind: kind, Value: value,
			Location:  &evidence.Location{Path: "batch.go", Line: line, Column: 3},
			Certainty: evidence.CertaintyStatic,
		}
	}
	symbol := func(id componentmap.MemberID, name string, line int) componentmap.Candidate {
		return componentmap.Candidate{
			ID: id, Role: componentmap.CandidateRoleConceptualMember, Name: name,
			ParentID: &fileID,
			Facts:    []componentmap.LocalFact{fact(componentmap.FactDeclaration, name, line)},
		}
	}
	return &ReportData{
		OpenablePaths: []string{"batch.go"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Version:                   ArchitectureCanvasVersion,
			LocalRemainderComponentID: "component-remainder",
			Components: []ArchitectureComponent{
				{
					ID: "component-symbols",
					Members: []componentmap.Candidate{
						{ID: packageID, Role: componentmap.CandidateRoleConceptualMember, Name: "service"},
						symbol(zebraID, "example.Service.Zebra", 30),
						symbol(alphaID, "example.Service.Alpha", 10),
					},
				},
				{
					ID: "component-package-only",
					Members: []componentmap.Candidate{{
						ID: packageOnlyID, Role: componentmap.CandidateRoleConceptualMember, Name: "package-only",
					}},
				},
				{
					ID: "component-remainder",
					Members: []componentmap.Candidate{
						symbol(remainderID, "example.Service.Remainder", 50),
					},
				},
			},
			StructuralLocators: []ArchitectureStructuralLocator{{
				Locator: componentmap.Candidate{
					ID: fileID, Role: componentmap.CandidateRoleStructuralLocator, Name: "batch.go",
					ParentID: &packageID,
					Facts:    []componentmap.LocalFact{fact(componentmap.FactRepositoryPath, "batch.go", 1)},
				},
				ParticipatingComponentIDs: []componentmap.ComponentID{
					"component-symbols", "component-remainder",
				},
			}},
		},
	}
}

func cloneArchitectureNavigationReport(t *testing.T, data *ReportData) *ReportData {
	t.Helper()
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var cloned ReportData
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatal(err)
	}
	return &cloned
}
