package orient

import (
	"testing"

	"github.com/dvordrova/repomap/internal/entrycall"
)

func TestDeliverEntryCallSubstrateHandsOffIndependentSnapshot(t *testing.T) {
	want := entrycall.Substrate{
		Version: entrycall.SubstrateVersion, State: entrycall.StateReady,
		Roots: []entrycall.ExactRoot{{NodeID: "root"}},
		SurfaceCandidates: []entrycall.ExactSurfaceCandidate{{
			ID: "candidate", RootNodeID: "root", Form: entrycall.SurfaceCandidateDirectCall,
			Sketch: "Handle", Site: entrycall.Location{Path: "routes.go", Line: 1},
			Facts: []entrycall.ExactSurfaceFact{{
				ID: "fact", Kind: entrycall.SurfaceFactString, Position: 1,
				Label: "path", Value: "/ready", Location: entrycall.Location{Path: "routes.go", Line: 1},
			}},
		}},
	}
	called := 0
	deliverEntryCallSubstrate(Options{EntryCallSubstrateSink: func(substrate entrycall.Substrate) {
		called++
		substrate.Roots[0].NodeID = "changed"
		substrate.SurfaceCandidates[0].Facts[0].Value = "changed"
	}}, nil)
	if called != 0 {
		t.Fatalf("nil substrate invoked sink %d time(s)", called)
	}
	deliverEntryCallSubstrate(Options{EntryCallSubstrateSink: func(substrate entrycall.Substrate) {
		called++
		substrate.Roots[0].NodeID = "changed"
		substrate.SurfaceCandidates[0].Facts[0].Value = "changed"
	}}, &want)
	if called != 1 {
		t.Fatalf("sink calls = %d, want 1", called)
	}
	if want.Roots[0].NodeID != "root" || want.SurfaceCandidates[0].Facts[0].Value != "/ready" {
		t.Fatalf("sink mutation changed producer substrate: %+v", want)
	}
	deliverEntryCallSubstrate(Options{}, &want)
}
