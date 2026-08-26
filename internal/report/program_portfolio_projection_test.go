package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestProgramPortfolioKeepsEveryExactTargetAndOneDefaultEntry(t *testing.T) {
	pythonIndex := reportProgramIndexFixture(t, "python", "executable")
	goIndex := reportProgramIndexFixture(t, "go", "library")
	portfolio, err := NewProgramPortfolio(
		pythonIndex.Target.ID,
		[]programindex.Index{pythonIndex, goIndex},
	)
	if err != nil {
		t.Fatalf("NewProgramPortfolio: %v", err)
	}
	if len(portfolio.Entries) != 2 || portfolio.Entries[0].Target.ID >= portfolio.Entries[1].Target.ID {
		t.Fatalf("portfolio entries are not a complete canonical set: %#v", portfolio.Entries)
	}
	semanticStates := make(map[string]ProgramSemanticState, len(portfolio.Entries))
	for _, entry := range portfolio.Entries {
		semanticStates[entry.Target.ID] = entry.SemanticState
	}
	if semanticStates[goIndex.Target.ID] != ProgramSemanticStructuralOnly ||
		semanticStates[pythonIndex.Target.ID] != ProgramSemanticProgramAvailable {
		t.Fatalf("portfolio semantic states = %#v", semanticStates)
	}
	defaultEntry, err := portfolio.defaultEntry()
	if err != nil {
		t.Fatal(err)
	}
	if defaultEntry.Target.ID != pythonIndex.Target.ID ||
		defaultEntry.View.TargetID != pythonIndex.Target.ID {
		t.Fatalf("default entry = %#v", defaultEntry)
	}
}

func TestProgramPortfolioRejectsDuplicateTargetEntries(t *testing.T) {
	index := reportProgramIndexFixture(t, "python", "library")
	if _, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index, index}); err == nil ||
		!strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("duplicate target error = %v", err)
	}
}

func TestProgramSemanticPresentationFailsClosedByLanguageCapability(t *testing.T) {
	pythonIndex := reportProgramIndexFixture(t, "python", "executable")
	pythonPortfolio, err := NewProgramPortfolio(
		pythonIndex.Target.ID,
		[]programindex.Index{pythonIndex},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateProgramSemanticPresentation(
		pythonPortfolio, nil, nil, nil, nil, nil, nil,
	); err == nil || !strings.Contains(err.Error(), "requires exact core map, activity entrypoint, integration usage, and activity path authority") {
		t.Fatalf("missing Python core-map authority error = %v", err)
	}
	if err := validateProgramSemanticPresentation(
		pythonPortfolio, nil, &CubeMapView{}, nil, nil, nil, nil,
	); err == nil || !strings.Contains(err.Error(), "requires exact core map, activity entrypoint, integration usage, and activity path authority") {
		t.Fatalf("Python cube-map authority error = %v", err)
	}
	if err := validateProgramSemanticPresentation(
		pythonPortfolio, nil, nil, &CoreMapView{}, nil, nil, nil,
	); err == nil || !strings.Contains(err.Error(), "requires exact core map, activity entrypoint, integration usage, and activity path authority") {
		t.Fatalf("missing Python integration-usage authority error = %v", err)
	}
	if err := validateProgramSemanticPresentation(
		pythonPortfolio, nil, nil, &CoreMapView{}, &ActivityEntrypointView{}, &IntegrationUsageView{},
		&ActivityPathView{},
	); err != nil {
		t.Fatalf("exact Python semantic capability topology: %v", err)
	}

	tampered := *pythonPortfolio
	tampered.Entries = append([]ProgramPortfolioEntry(nil), pythonPortfolio.Entries...)
	tampered.Entries[0].SemanticState = ProgramSemanticAvailable
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "invalid semantic state") {
		t.Fatalf("tampered semantic state error = %v", err)
	}

	goIndex := reportProgramIndexFixture(t, "go", "library")
	goPortfolio, err := NewProgramPortfolio(goIndex.Target.ID, []programindex.Index{goIndex})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateProgramSemanticPresentation(
		goPortfolio, nil, nil, nil, nil, nil, nil,
	); err == nil || !strings.Contains(err.Error(), "requires exact core map, activity entrypoint, integration usage, and activity path authority") {
		t.Fatalf("missing Go semantic authority error = %v", err)
	}
}

func TestProgramSemanticPresentationRequiresExactJSTSSurfaceAuthority(t *testing.T) {
	for _, language := range []string{"javascript", "typescript"} {
		t.Run(language, func(t *testing.T) {
			index := reportProgramIndexFixture(t, language, "application")
			portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
			if err != nil {
				t.Fatal(err)
			}
			if portfolio.Entries[0].SemanticState != ProgramSemanticProgramAvailable {
				t.Fatalf("semantic state = %q", portfolio.Entries[0].SemanticState)
			}
			if err := validateProgramSemanticPresentation(
				portfolio, nil, nil, &CoreMapView{}, &ActivityEntrypointView{},
				&IntegrationUsageView{}, &ActivityPathView{},
			); err == nil || !strings.Contains(err.Error(), "requires exact surface catalog and cross-surface path authority") {
				t.Fatalf("missing JavaScript/TypeScript authority error = %v", err)
			}
			if err := validateProgramSemanticPresentation(
				portfolio, nil, nil, &CoreMapView{}, &ActivityEntrypointView{},
				&IntegrationUsageView{}, &ActivityPathView{},
				jstsSemanticPresentation{&JSTSSurfaceCatalogView{}, &CrossSurfacePathView{}},
			); err != nil {
				t.Fatalf("complete JavaScript/TypeScript semantic topology: %v", err)
			}
		})
	}
}
