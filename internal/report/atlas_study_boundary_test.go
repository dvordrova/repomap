package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/repositoryatlas/goadapter"
)

// TestAtlasStudyBoundaryCallSiteBecomesReadingTarget verifies the D214 Study
// consumer: an observed resource-boundary call site (adapter-owned Atlas
// boundary evidence) becomes a reading target with the observed-call-boundary
// support role and produces a focused route span.
func TestAtlasStudyBoundaryCallSiteBecomesReadingTarget(t *testing.T) {
	data := atlasStudyReportFixture(t)
	openablePath := "cmd/app/main.go"

	// The fixture Atlas carries no package unit; add one so the boundary
	// evidence binds to a real package unit (parented under the module).
	packageUnitID := "unit-package-fixture"
	moduleUnitID := ""
	for _, unit := range data.RepositoryAtlas.Units {
		if unit.Kind == repositoryatlas.UnitModule {
			moduleUnitID = unit.ID
			break
		}
	}
	if moduleUnitID == "" {
		t.Fatal("fixture Atlas has no module unit")
	}
	data.RepositoryAtlas.Units = append(data.RepositoryAtlas.Units, repositoryatlas.Unit{
		ID: packageUnitID, Kind: repositoryatlas.UnitPackage,
		ParentID: moduleUnitID, Name: "example.com/fixture/cmd/app",
	})
	// RepositoryGraph package owning the openable path supplies the exact
	// package bucket for the observed-call-boundary support proof.
	data.RepositoryGraph = &RepositoryGraph{
		Version: 1,
		Packages: []PackageInfo{{
			CanonicalPath: "example.com/fixture/cmd/app", Name: "main",
			Files: []string{openablePath},
		}},
	}
	unitID := ""
	for _, unit := range data.RepositoryAtlas.Units {
		if unit.Kind == repositoryatlas.UnitPackage {
			unitID = unit.ID
			break
		}
	}
	if unitID == "" {
		t.Fatal("fixture Atlas has no package unit")
	}
	evidenceID := "evidence-boundary-sql-open"
	data.RepositoryAtlas.Evidence = append(data.RepositoryAtlas.Evidence, repositoryatlas.Evidence{
		ID: evidenceID, UnitID: unitID,
		Location: evidence.Location{Path: openablePath, Line: 40, Column: 5},
		Symbol:   "main",
		Provenance: evidence.Provenance{
			Provider:  goadapter.BoundaryObservationEvidenceProvider,
			Version:   goadapter.BoundaryObservationEvidenceVersion,
			Operation: goadapter.BoundaryObservationEvidenceOperation,
			Detail:    "database/sql",
		},
	})
	data.RepositoryAtlas.Observations = append(data.RepositoryAtlas.Observations, repositoryatlas.Observation{
		ID: "observation-boundary-sql-open", UnitID: unitID,
		Subject:      repositoryatlas.EntityRef{Kind: repositoryatlas.EntityBoundary, ID: "boundary-fixture"},
		EvidenceRefs: []string{evidenceID},
	})
	data.RepositoryAtlas.Entities = append(data.RepositoryAtlas.Entities, repositoryatlas.Entity{
		ID: "boundary-fixture", Kind: repositoryatlas.EntityBoundary, UnitID: unitID,
	})
	if err := data.RepositoryAtlas.Validate(); err != nil {
		t.Fatalf("fixture Atlas with boundary evidence: %v", err)
	}

	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}

	foundTarget := false
	for _, target := range input.ReadingTargets {
		if target.Location.Path != openablePath || target.Location.Line != 40 {
			continue
		}
		foundTarget = true
		if target.Symbol != "main" || target.Kind != atlasstudy.ReadingTargetFunction {
			t.Fatalf("boundary reading target = %#v", target)
		}
	}
	if !foundTarget {
		t.Fatalf("boundary call site did not become a reading target; targets:\n%+v", input.ReadingTargets)
	}

	foundSpan := false
	for _, span := range input.RouteSpans {
		if span.Kind != atlasstudy.RouteSpanFocused {
			continue
		}
		for _, supportID := range span.RequiredSupportIDs {
			for _, support := range input.ReadingSupports {
				if support.ID != supportID || support.Role != atlasstudy.SupportObservedCallBoundary {
					continue
				}
				foundSpan = true
			}
		}
	}
	if !foundSpan {
		t.Fatalf("boundary call site produced no observed-call-boundary focused span; spans:\n%+v", input.RouteSpans)
	}
}
