package orient

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

func TestAddResearchFocusLocationsUsesLocalEvidencePriority(t *testing.T) {
	t.Parallel()

	const filePath = "internal/server/server.go"
	serverStart := surfacediscovery.Location{Path: filePath, Line: 71}
	descriptor := surfacediscovery.Location{Path: filePath, Line: 72}
	surfaceResult := &surfacediscovery.Result{
		Catalog: surfacediscovery.TriggerCatalog{Triggers: []surfacediscovery.TriggerRecord{{
			RegistrationSite:  surfacediscovery.Location{Path: filePath, Line: 70},
			ServerStartSite:   &serverStart,
			DescriptorSite:    &descriptor,
			ProcessEntrypoint: surfacediscovery.Symbol{Location: surfacediscovery.Location{Path: filePath, Line: 73}},
		}}},
		Grounding: surfacediscovery.ArchitectureGrounding{Anchors: []surfacediscovery.BehaviorAnchor{{
			Location: surfacediscovery.Location{Path: filePath, Line: 50},
		}}},
	}
	report := &orientationPart{CandidateFlows: []flowexplain.CandidateFlow{{
		LocalProof: &flowproof.Session{Proof: flowproof.Proof{
			Anchors: []flowproof.Anchor{{
				ID: "handler", Location: &evidence.Location{Path: filePath, Line: 62},
			}},
			Transitions: []flowproof.Transition{
				{ID: "frontier", Resolution: evidence.ResolutionUnresolved, Evidence: evidence.Location{Path: filePath, Line: 60}},
				{ID: "resolved", Resolution: evidence.ResolutionStatic, Evidence: evidence.Location{Path: filePath, Line: 61}},
			},
			Slots: []flowproof.Slot{{
				Status: flowproof.SlotUnresolved, EvidenceIDs: []string{"frontier"},
			}},
		}},
	}}}
	snap := snapshot.Snapshot{GoFacts: &gofacts.Facts{CommandTraces: []gofacts.CommandTrace{{
		Steps: []gofacts.CommandTraceStep{{
			CallsiteLocation: &evidence.Location{Path: filePath, Line: 41},
			TargetLocation:   evidence.Location{Path: filePath, Line: 39},
		}},
		HandlerCalls: []gofacts.CommandTraceCall{{
			Path: filePath, Line: 40, Resolved: true, TargetPath: filePath, TargetLine: 38,
		}},
	}}}}

	candidates := addResearchFocusLocations(
		[]modelresearch.FileCandidate{{ID: "server", Path: filePath}},
		report,
		surfaceResult,
		snap,
		[]sourcesignals.Signal{{Path: filePath, Line: 30}},
	)
	got := make([]int, 0, len(candidates[0].FocusLocations))
	for _, location := range candidates[0].FocusLocations {
		got = append(got, location.Line)
	}
	want := []int{70, 71, 72, 73, 60, 61, 62, 50, 40, 41, 38, 39, 30}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("focus priority = %v, want %v", got, want)
	}
}

func TestSavedFlowProofIDsExcludeEvidenceOnlyDirections(t *testing.T) {
	t.Parallel()

	flows := []flowexplain.CandidateFlow{
		{Name: "Evidence only", Disposition: flowexplain.DirectionAccepted},
		{
			Name: "Saved trace", Disposition: flowexplain.DirectionAccepted,
			LocalProof: &flowproof.Session{Proof: flowproof.Proof{ID: "saved-trace"}},
		},
	}
	if got := savedFlowProofIDs(flows); !reflect.DeepEqual(got, []string{"saved-trace"}) {
		t.Fatalf("savedFlowProofIDs() = %v", got)
	}
}
