package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
)

func TestLinkArchitectureProductObjectsUsesExactEvidenceJoins(t *testing.T) {
	t.Parallel()

	const componentID = componentmap.ComponentID("component-cli")
	data := &ReportData{
		CandidateDirections: []CandidateDirection{
			{ID: "backup", Name: "Backup", WhyInteresting: "Primary user operation"},
			{ID: "check", Name: "Check", LikelyEntrypoint: "cmd/restic/cmd_check.go"},
		},
		ArchitectureCanvas: &ArchitectureCanvas{
			Components: []ArchitectureComponent{{
				ID: componentID,
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "github.com/restic/restic/cmd/restic"},
					Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactRepositoryPath, Value: "cmd/restic",
						Location: &evidence.Location{Path: "cmd/restic", Line: 1}, Certainty: evidence.CertaintyStatic,
					}},
				}},
				ParticipatingFlowIDs: []componentmap.FlowID{"backup"},
			}},
			Flows: []ArchitectureFlow{{
				ID: "backup", Name: "Backup", Trigger: "CLI command backup", Command: "backup",
				Steps: []ArchitectureFlowStep{{
					ID: "register-backup", ComponentID: componentID,
					Location: &evidence.Location{Path: "cmd/restic/cmd_backup.go", Line: 74},
				}},
				Slots: []flowproof.Slot{
					{Kind: flowproof.SlotTrigger, Status: flowproof.SlotVerified, EvidenceIDs: []string{"register-backup"}},
					{Kind: flowproof.SlotEntrypoint, Status: flowproof.SlotVerified, EvidenceIDs: []string{"register-backup"}},
				},
			}},
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "surface-backup", Kind: "worker", Identity: SurfaceIdentity{Name: "backup"},
			ProcessEntrypoint: SurfaceSymbol{Package: "github.com/restic/restic/cmd/restic", Location: &SurfaceLocation{Path: "cmd/restic/main.go", Line: 10}},
			RegistrationSite:  &SurfaceLocation{Path: "cmd/restic/cmd_backup.go", Line: 74},
			Handler:           SurfaceValue{Known: true, Text: "runBackup"}, Certainty: "static", Resolution: "static",
		}}},
	}

	linkArchitectureProductObjects(data)

	component := data.ArchitectureCanvas.Components[0]
	if len(component.OwnedSurfaceIDs) != 1 || component.OwnedSurfaceIDs[0] != "surface-backup" {
		t.Fatalf("owned surfaces = %v", component.OwnedSurfaceIDs)
	}
	if len(component.SuggestedInvestigationIDs) != 1 || component.SuggestedInvestigationIDs[0] != "check" {
		t.Fatalf("suggested investigations = %v", component.SuggestedInvestigationIDs)
	}
	surface := data.ArchitectureCanvas.Surfaces[0]
	if surface.OwningComponentID != componentID || surface.RelatedTraceID != "backup" {
		t.Fatalf("surface join = %#v", surface)
	}
	trace := data.ArchitectureCanvas.Flows[0]
	if trace.Status != "complete" || trace.GroundedAreas != 2 || trace.TotalAreas != 2 ||
		trace.StartSurfaceID != surface.ID || len(trace.ParticipatingComponentIDs) != 1 {
		t.Fatalf("trace summary = %#v", trace)
	}
}

func TestLinkArchitectureProductObjectsLeavesAmbiguousSurfaceUnassigned(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		ArchitectureCanvas: &ArchitectureCanvas{Components: []ArchitectureComponent{
			architecturePathComponent("a", "shared/start.go"),
			architecturePathComponent("b", "shared/start.go"),
		}},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "shared", Kind: "async_task", Identity: SurfaceIdentity{Name: "shared"},
			RegistrationSite: &SurfaceLocation{Path: "shared/start.go", Line: 20},
			Handler:          SurfaceValue{Known: true, Text: "start"}, Certainty: "static", Resolution: "static",
		}}},
	}

	linkArchitectureProductObjects(data)

	surface := data.ArchitectureCanvas.Surfaces[0]
	if surface.OwningComponentID != "" || surface.Category != surfaceCategoryUnassigned {
		t.Fatalf("ambiguous surface = %#v", surface)
	}
	if surface.TraceUnavailableReason != "unsupported surface kind" {
		t.Fatalf("unavailable reason = %q", surface.TraceUnavailableReason)
	}
}

func TestRefreshProductCountsKeepsSuggestionsDistinctFromSavedTraces(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		Run: &RunInfo{},
		CandidateDirections: []CandidateDirection{
			{ID: "complete"},
			{ID: "suggested"},
			{ID: "rejected", Disposition: flowexplain.DirectionRejected},
		},
		ArchitectureCanvas: &ArchitectureCanvas{
			Surfaces: []ArchitectureSurface{{ID: "one"}, {ID: "two"}},
			Flows:    []ArchitectureFlow{{ID: "complete", Status: "complete"}, {ID: "partial", Status: "partial"}},
		},
		Flows: []FlowData{{ID: "failed", Error: "bounded analysis failed"}},
	}

	refreshProductCounts(data)

	if data.Run.SuggestedInvestigationCount != 1 || data.Run.SavedTraceCount != 2 ||
		data.Run.CompleteTraceCount != 1 || data.Run.PartialTraceCount != 1 ||
		data.Run.FailedTraceAttemptCount != 1 || data.Run.DiscoveredSurfaceCount != 2 {
		t.Fatalf("counts = %#v", data.Run)
	}
}

func architecturePathComponent(id componentmap.ComponentID, repositoryPath string) ArchitectureComponent {
	return ArchitectureComponent{
		ID: id,
		Members: []componentmap.Candidate{{
			ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: repositoryPath},
			Facts: []componentmap.LocalFact{{
				Kind: componentmap.FactRepositoryPath, Value: repositoryPath,
				Location: &evidence.Location{Path: repositoryPath, Line: 1}, Certainty: evidence.CertaintyStatic,
			}},
		}},
	}
}
